// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

//! Convenience Network API for Diem

pub use crate::protocols::rpc::error::RpcError;
use crate::{
    error::NetworkError,
    peer_manager::{
        ConnectionNotification, ConnectionRequestSender, PeerManagerNotification,
        PeerManagerRequestSender,
    },
    transport::ConnectionMetadata,
    ProtocolId,
};
use async_trait::async_trait;
use bytes::Bytes;
use channel::diem_channel;
use diem_logger::prelude::*;
use diem_types::{network_address::NetworkAddress, PeerId};
use futures::{
    channel::oneshot,
    future,
    stream::{FilterMap, FusedStream, Map, Select, Stream, StreamExt},
    task::{Context, Poll},
};
use pin_project::pin_project;
use serde::{de::DeserializeOwned, Deserialize, Serialize};
use short_hex_str::AsShortHexStr;
use std::{cmp::min, iter::FromIterator, marker::PhantomData, pin::Pin, time::Duration};

use super::wire::handshake::v1::ProtocolIdSet;
use std::fmt::Debug;

pub trait Message: DeserializeOwned + Serialize {}
impl<T: DeserializeOwned + Serialize> Message for T {}

/// Events received by network clients in a validator
///
/// An enumeration of the various types of messages that the network will be sending
/// to its clients. This differs from [`PeerNotification`] since the contents are deserialized
/// into the type `TMessage` over which `Event` is generic. Note that we assume here that for every
/// consumer of this API there's a singleton message type, `TMessage`,  which encapsulates all the
/// messages and RPCs that are received by that consumer.
///
/// [`PeerNotification`]: crate::peer::PeerNotification
#[derive(Debug)]
pub enum Event<TMessage> {
    /// New inbound direct-send message from peer.
    Message(PeerId, TMessage),
    /// New inbound rpc request. The request is fulfilled by sending the
    /// serialized response `Bytes` over the `oneshot::Sender`, where the network
    /// layer will handle sending the response over-the-wire.
    RpcRequest(
        PeerId,
        TMessage,
        ProtocolId,
        oneshot::Sender<Result<Bytes, RpcError>>,
    ),
    /// Peer which we have a newly established connection with.
    NewPeer(ConnectionMetadata),
    /// Peer with which we've lost our connection.
    LostPeer(ConnectionMetadata),
}

/// impl PartialEq for simpler testing
impl<TMessage: PartialEq> PartialEq for Event<TMessage> {
    fn eq(&self, other: &Event<TMessage>) -> bool {
        use Event::*;
        match (self, other) {
            (Message(pid1, msg1), Message(pid2, msg2)) => pid1 == pid2 && msg1 == msg2,
            // ignore oneshot::Sender in comparison
            (RpcRequest(pid1, msg1, proto1, _), RpcRequest(pid2, msg2, proto2, _)) => {
                pid1 == pid2 && msg1 == msg2 && proto1 == proto2
            }
            (NewPeer(metadata1), NewPeer(metadata2)) => metadata1 == metadata2,
            (LostPeer(metadata1), LostPeer(metadata2)) => metadata1 == metadata2,
            _ => false,
        }
    }
}

/// Configuration needed for DiemNet applications to register with the network
/// builder. Supports client-only, service-only, and p2p (both) applications.
// TODO(philiphayes): separate configs for client & server?
#[derive(Clone, Default)]
pub struct AppConfig {
    /// The set of protocols needed for this application.
    pub protocols: ProtocolIdSet,
    /// The config for the inbound message queue from network to the application.
    /// Used for specifying the queue style (e.g. FIFO vs LIFO) and sub-queue max
    /// capacity.
    // TODO(philiphayes): only relevant for services
    // TODO(philiphayes): in the future, use a Service trait here instead?
    pub inbound_queue: Option<diem_channel::Config>,
}

impl AppConfig {
    /// DiemNet client configuration. Requires the set of protocols used by the
    /// client in its requests.
    pub fn client(protocols: impl IntoIterator<Item = ProtocolId>) -> Self {
        Self {
            protocols: ProtocolIdSet::from_iter(protocols),
            inbound_queue: None,
        }
    }

    /// DiemNet service configuration. Requires both the set of protocols this
    /// service can handle and the queue configuration.
    pub fn service(
        protocols: impl IntoIterator<Item = ProtocolId>,
        inbound_queue: diem_channel::Config,
    ) -> Self {
        Self {
            protocols: ProtocolIdSet::from_iter(protocols),
            inbound_queue: Some(inbound_queue),
        }
    }

    /// DiemNet peer-to-peer service configuration. A peer-to-peer service is both
    /// a client and a service.
    pub fn p2p(
        protocols: impl IntoIterator<Item = ProtocolId>,
        inbound_queue: diem_channel::Config,
    ) -> Self {
        Self {
            protocols: ProtocolIdSet::from_iter(protocols),
            inbound_queue: Some(inbound_queue),
        }
    }
}

/// A `Stream` of `Event<TMessage>` from the lower network layer to an upper
/// network application that deserializes inbound network direct-send and rpc
/// messages into `TMessage`. Inbound messages that fail to deserialize are logged
/// and dropped.
///
/// `NetworkEvents` is really just a thin wrapper around a
/// `channel::Receiver<PeerNotification>` that deserializes inbound messages.
#[pin_project]
pub struct NetworkEvents<TMessage> {
    #[pin]
    event_stream: Select<
        FilterMap<
            diem_channel::Receiver<(PeerId, ProtocolId), PeerManagerNotification>,
            future::Ready<Option<Event<TMessage>>>,
            fn(PeerManagerNotification) -> future::Ready<Option<Event<TMessage>>>,
        >,
        Map<
            diem_channel::Receiver<PeerId, ConnectionNotification>,
            fn(ConnectionNotification) -> Event<TMessage>,
        >,
    >,
    _marker: PhantomData<TMessage>,
}

/// Trait specifying the signature for `new()` `NetworkEvents`
pub trait NewNetworkEvents {
    fn new(
        peer_mgr_notifs_rx: diem_channel::Receiver<(PeerId, ProtocolId), PeerManagerNotification>,
        connection_notifs_rx: diem_channel::Receiver<PeerId, ConnectionNotification>,
    ) -> Self;
}

impl<TMessage: Message> NewNetworkEvents for NetworkEvents<TMessage> {
    fn new(
        peer_mgr_notifs_rx: diem_channel::Receiver<(PeerId, ProtocolId), PeerManagerNotification>,
        connection_notifs_rx: diem_channel::Receiver<PeerId, ConnectionNotification>,
    ) -> Self {
        let data_event_stream = peer_mgr_notifs_rx.filter_map(
            peer_mgr_notif_to_event
                as fn(PeerManagerNotification) -> future::Ready<Option<Event<TMessage>>>,
        );
        let control_event_stream = connection_notifs_rx
            .map(control_msg_to_event as fn(ConnectionNotification) -> Event<TMessage>);
        Self {
            event_stream: ::futures::stream::select(data_event_stream, control_event_stream),
            _marker: PhantomData,
        }
    }
}

impl<TMessage> Stream for NetworkEvents<TMessage> {
    type Item = Event<TMessage>;

    fn poll_next(self: Pin<&mut Self>, context: &mut Context) -> Poll<Option<Self::Item>> {
        self.project().event_stream.poll_next(context)
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        self.event_stream.size_hint()
    }
}

/// Deserialize inbound direct send and rpc messages into the application `TMessage`
/// type, logging and dropping messages that fail to deserialize.
fn peer_mgr_notif_to_event<TMessage: Message>(
    notif: PeerManagerNotification,
) -> future::Ready<Option<Event<TMessage>>> {
    let maybe_event = match notif {
        PeerManagerNotification::RecvRpc(peer_id, rpc_req) => {
            request_to_network_event(peer_id, &rpc_req)
                .map(|msg| Event::RpcRequest(peer_id, msg, rpc_req.protocol_id, rpc_req.res_tx))
        }
        PeerManagerNotification::RecvMessage(peer_id, request) => {
            request_to_network_event(peer_id, &request).map(|msg| Event::Message(peer_id, msg))
        }
    };
    future::ready(maybe_event)
}

/// Converts a `SerializedRequest` into a network `Event` for sending to other nodes
fn request_to_network_event<TMessage: Message, Request: SerializedRequest>(
    peer_id: PeerId,
    request: &Request,
) -> Option<TMessage> {
    match request.to_message() {
        Ok(msg) => Some(msg),
        Err(err) => {
            let data = &request.data();
            warn!(
                SecurityEvent::InvalidNetworkEvent,
                error = ?err,
                remote_peer_id = peer_id.short_str(),
                protocol_id = request.protocol_id(),
                data_prefix = hex::encode(&data[..min(16, data.len())]),
            );
            None
        }
    }
}

fn control_msg_to_event<TMessage>(notif: ConnectionNotification) -> Event<TMessage> {
    match notif {
        ConnectionNotification::NewPeer(metadata, _context) => Event::NewPeer(metadata),
        ConnectionNotification::LostPeer(metadata, _context, _reason) => Event::LostPeer(metadata),
    }
}

impl<TMessage> FusedStream for NetworkEvents<TMessage> {
    fn is_terminated(&self) -> bool {
        self.event_stream.is_terminated()
    }
}

/// `NetworkSender` is the generic interface from upper network applications to
/// the lower network layer. It provides the full API for network applications,
/// including sending direct-send messages, sending rpc requests, as well as
/// dialing or disconnecting from peers and updating the list of accepted public
/// keys.
///
/// `NetworkSender` is in fact a thin wrapper around a `PeerManagerRequestSender`, which in turn is
/// a thin wrapper on `diem_channel::Sender<(PeerId, ProtocolId), PeerManagerRequest>`,
/// mostly focused on providing a more ergonomic API. However, network applications will usually
/// provide their own thin wrapper around `NetworkSender` that narrows the API to the specific
/// interface they need. For instance, `mempool` only requires direct-send functionality so its
/// `MempoolNetworkSender` only exposes a `send_to` function.
///
/// Provide Protobuf wrapper over `[peer_manager::PeerManagerRequestSender]`
#[derive(Clone, Debug)]
pub struct NetworkSender<TMessage> {
    peer_mgr_reqs_tx: PeerManagerRequestSender,
    connection_reqs_tx: ConnectionRequestSender,
    _marker: PhantomData<TMessage>,
}

/// Trait specifying the signature for `new()` `NetworkSender`s
pub trait NewNetworkSender {
    fn new(
        peer_mgr_reqs_tx: PeerManagerRequestSender,
        connection_reqs_tx: ConnectionRequestSender,
    ) -> Self;
}

impl<TMessage> NewNetworkSender for NetworkSender<TMessage> {
    fn new(
        peer_mgr_reqs_tx: PeerManagerRequestSender,
        connection_reqs_tx: ConnectionRequestSender,
    ) -> Self {
        Self {
            peer_mgr_reqs_tx,
            connection_reqs_tx,
            _marker: PhantomData,
        }
    }
}

impl<TMessage> NetworkSender<TMessage> {
    /// Request that a given Peer be dialed at the provided `NetworkAddress` and
    /// synchronously wait for the request to be performed.
    pub async fn dial_peer(&self, peer: PeerId, addr: NetworkAddress) -> Result<(), NetworkError> {
        self.connection_reqs_tx.dial_peer(peer, addr).await?;
        Ok(())
    }

    /// Request that a given Peer be disconnected and synchronously wait for the request to be
    /// performed.
    pub async fn disconnect_peer(&self, peer: PeerId) -> Result<(), NetworkError> {
        self.connection_reqs_tx.disconnect_peer(peer).await?;
        Ok(())
    }
}

impl<TMessage: Message> NetworkSender<TMessage> {
    /// Send a protobuf message to a single recipient. Provides a wrapper over
    /// `[peer_manager::PeerManagerRequestSender::send_to]`.
    pub fn send_to(
        &self,
        recipient: PeerId,
        protocol: ProtocolId,
        message: TMessage,
    ) -> Result<(), NetworkError> {
        let mdata = protocol.to_bytes(&message)?.into();
        self.peer_mgr_reqs_tx.send_to(recipient, protocol, mdata)?;
        Ok(())
    }

    /// Send a protobuf message to a many recipients. Provides a wrapper over
    /// `[peer_manager::PeerManagerRequestSender::send_to_many]`.
    pub fn send_to_many(
        &self,
        recipients: impl Iterator<Item = PeerId>,
        protocol: ProtocolId,
        message: TMessage,
    ) -> Result<(), NetworkError> {
        // Serialize message.
        let mdata = protocol.to_bytes(&message)?.into();
        self.peer_mgr_reqs_tx
            .send_to_many(recipients, protocol, mdata)?;
        Ok(())
    }

    /// Send a protobuf rpc request to a single recipient while handling
    /// serialization and deserialization of the request and response respectively.
    /// Assumes that the request and response both have the same message type.
    pub async fn send_rpc(
        &self,
        recipient: PeerId,
        protocol: ProtocolId,
        req_msg: TMessage,
        timeout: Duration,
    ) -> Result<TMessage, RpcError> {
        // serialize request
        let req_data = protocol.to_bytes(&req_msg)?.into();
        let res_data = self
            .peer_mgr_reqs_tx
            .send_rpc(recipient, protocol, req_data, timeout)
            .await?;
        let res_msg: TMessage = protocol.from_bytes(&res_data)?;
        Ok(res_msg)
    }
}

/// A simplified version of `NetworkSender` that doesn't use `ProtocolId` in the input
/// It was already being implemented for every application, but is now standardized
#[async_trait]
pub trait ApplicationNetworkSender<TMessage: Send>: Clone {
    fn send_to(&self, _recipient: PeerId, _message: TMessage) -> Result<(), NetworkError> {
        unimplemented!()
    }

    fn send_to_many(
        &self,
        _recipients: impl Iterator<Item = PeerId>,
        _message: TMessage,
    ) -> Result<(), NetworkError> {
        unimplemented!()
    }

    async fn send_rpc(
        &self,
        recipient: PeerId,
        req_msg: TMessage,
        timeout: Duration,
    ) -> Result<TMessage, RpcError>;
}

/// Generalized functionality for any request across `DirectSend` and `Rpc`.
pub trait SerializedRequest {
    fn protocol_id(&self) -> ProtocolId;
    fn data(&self) -> &Bytes;

    /// Converts the `SerializedMessage` into its deserialized version of `TMessage` based on the
    /// `ProtocolId`.  See: [`ProtocolId::from_bytes`]
    fn to_message<'a, TMessage: Deserialize<'a>>(&'a self) -> anyhow::Result<TMessage> {
        self.protocol_id().from_bytes(self.data())
    }
}
