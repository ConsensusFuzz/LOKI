// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use diem_types::transaction::Version;
use futures::channel::{mpsc::SendError, oneshot::Canceled};
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Debug, Deserialize, Error, PartialEq, Serialize)]
pub enum Error {
    #[error("State sync has not yet finished bootstrapping! Error: {0}")]
    BootstrapNotComplete(String),
    #[error("Failed to send callback: {0}")]
    CallbackSendFailed(String),
    #[error("Error encountered in the event subscription service: {0}")]
    EventNotificationError(String),
    #[error("A consensus notification was sent to a full node: {0}")]
    FullNodeConsensusNotification(String),
    #[error("An integer overflow has occurred: {0}")]
    IntegerOverflow(String),
    #[error("Failed to notify mempool of the new commit: {0}")]
    NotifyMempoolError(String),
    #[error("Received an old sync request for version {0}, but our committed version is: {1}")]
    OldSyncRequest(Version, Version),
    #[error("Received oneshot::canceled. The sender of a channel was dropped: {0}")]
    SenderDroppedError(String),
    #[error("Unexpected storage error: {0}")]
    StorageError(String),
    #[error("Synced beyond the target version. Committed version: {0}, target version: {1}")]
    SyncedBeyondTarget(Version, Version),
    #[error("Unexpected error: {0}")]
    UnexpectedError(String),
}

impl From<SendError> for Error {
    fn from(error: SendError) -> Self {
        Error::UnexpectedError(error.to_string())
    }
}

impl From<Canceled> for Error {
    fn from(canceled: Canceled) -> Self {
        Error::SenderDroppedError(canceled.to_string())
    }
}
