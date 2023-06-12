// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

//! Interface between Mempool and Network layers.

use crate::counters;
use channel::message_queues::QueueStyle;
use diem_metrics::IntCounterVec;
use diem_types::{transaction::SignedTransaction, PeerId};
use fail::fail_point;
use network::{
    error::NetworkError,
    peer_manager::{ConnectionRequestSender, PeerManagerRequestSender},
    protocols::network::{NetworkEvents, NetworkSender, NewNetworkSender},
    ProtocolId,
};
use serde::{Deserialize, Serialize};

/// Container for exchanging transactions with other Mempools.
#[derive(Clone, Debug, Deserialize, Serialize)]
pub enum MempoolSyncMsg {
    /// Broadcast request issued by the sender.
    BroadcastTransactionsRequest {
        /// Unique id of sync request. Can be used by sender for rebroadcast analysis
        request_id: Vec<u8>,
        transactions: Vec<SignedTransaction>,
    },
    /// Broadcast ack issued by the receiver.
    BroadcastTransactionsResponse {
        request_id: Vec<u8>,
        /// Retry signal from recipient if there are txns in corresponding broadcast
        /// that were rejected from mempool but may succeed on resend.
        retry: bool,
        /// A backpressure signal from the recipient when it is overwhelmed (e.g., mempool is full).
        backoff: bool,
    },
}

/// Protocol id for mempool direct-send calls.
pub const MEMPOOL_DIRECT_SEND_PROTOCOL: &[u8] = b"/diem/direct-send/0.1.0/mempool/0.1.0";

/// The interface from Network to Mempool layer.
///
/// `MempoolNetworkEvents` is a `Stream` of `PeerManagerNotification` where the
/// raw `Bytes` direct-send and rpc messages are deserialized into
/// `MempoolMessage` types. `MempoolNetworkEvents` is a thin wrapper around an
/// `channel::Receiver<PeerManagerNotification>`.
pub type MempoolNetworkEvents = NetworkEvents<MempoolSyncMsg>;

/// The interface from Mempool to Networking layer.
///
/// This is a thin wrapper around a `NetworkSender<MempoolSyncMsg>`, so it is
/// easy to clone and send off to a separate task. For example, the rpc requests
/// return Futures that encapsulate the whole flow, from sending the request to
/// remote, to finally receiving the response and deserializing. It therefore
/// makes the most sense to make the rpc call on a separate async task, which
/// requires the `MempoolNetworkSender` to be `Clone` and `Send`.
#[derive(Clone)]
pub struct MempoolNetworkSender {
    inner: NetworkSender<MempoolSyncMsg>,
}

/// Create a new Sender that only sends for the `MEMPOOL_DIRECT_SEND_PROTOCOL` ProtocolId and a
/// Receiver (Events) that explicitly returns only said ProtocolId..
pub fn network_endpoint_config(
    max_broadcasts_per_peer: usize,
) -> (
    Vec<ProtocolId>,
    Vec<ProtocolId>,
    QueueStyle,
    usize,
    Option<&'static IntCounterVec>,
) {
    (
        vec![],
        vec![ProtocolId::MempoolDirectSend],
        QueueStyle::KLAST,
        max_broadcasts_per_peer,
        Some(&counters::PENDING_MEMPOOL_NETWORK_EVENTS),
    )
}

impl NewNetworkSender for MempoolNetworkSender {
    fn new(
        peer_mgr_reqs_tx: PeerManagerRequestSender,
        connection_reqs_tx: ConnectionRequestSender,
    ) -> Self {
        Self {
            inner: NetworkSender::new(peer_mgr_reqs_tx, connection_reqs_tx),
        }
    }
}

impl MempoolNetworkSender {
    pub fn send_to(
        &mut self,
        recipient: PeerId,
        message: MempoolSyncMsg,
    ) -> Result<(), NetworkError> {
        fail_point!("mempool::send_to", |_| {
            Err(anyhow::anyhow!("Injected error in mempool::send_to").into())
        });
        let protocol = ProtocolId::MempoolDirectSend;
        self.inner.send_to(recipient, protocol, message)
    }
}
