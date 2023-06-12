// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    error::StateSyncError, experimental::execution_phase::ExecutionChannelType,
    state_replication::StateComputer,
};
use anyhow::Result;
use channel::Sender;
use consensus_types::{block::Block, executed_block::ExecutedBlock};
use diem_crypto::HashValue;
use diem_types::ledger_info::LedgerInfoWithSignatures;
use executor_types::{Error as ExecutionError, StateComputeResult};
use fail::fail_point;
use futures::SinkExt;
use std::{boxed::Box, sync::Arc};

use crate::state_replication::StateComputerCommitCallBackType;

/// Ordering-only execution proxy
/// implements StateComputer traits.
/// Used only when node_config.validator.consensus.decoupled = true.
pub struct OrderingStateComputer {
    // the channel to pour vectors of blocks into
    // the real execution phase (will be handled in ExecutionPhase).
    executor_channel: Sender<ExecutionChannelType>,
}

impl OrderingStateComputer {
    pub fn new(executor_channel: Sender<ExecutionChannelType>) -> Self {
        Self { executor_channel }
    }
}

#[async_trait::async_trait]
impl StateComputer for OrderingStateComputer {
    fn compute(
        &self,
        // The block to be executed.
        _block: &Block,
        // The parent block id.
        _parent_block_id: HashValue,
    ) -> Result<StateComputeResult, ExecutionError> {
        // Return dummy block and bypass the execution phase.
        // This will break the e2e smoke test (for now because
        // no one is actually handling the next phase) if the
        // decoupled execution feature is turned on.
        Ok(StateComputeResult::new_dummy())
    }

    /// Send ordered blocks to the real execution phase through the channel.
    /// A future is fulfilled right away when the blocks are sent into the channel.
    async fn commit(
        &self,
        blocks: &[Arc<ExecutedBlock>],
        finality_proof: LedgerInfoWithSignatures,
        callback: StateComputerCommitCallBackType,
    ) -> Result<(), ExecutionError> {
        let ordered_block = blocks.iter().map(|b| b.block().clone()).collect();

        self.executor_channel
            .clone()
            .send((ordered_block, finality_proof, callback))
            .await
            .map_err(|e| ExecutionError::InternalError {
                error: e.to_string(),
            })?;
        Ok(())
    }

    /// Synchronize to a commit that not present locally.
    async fn sync_to(&self, _target: LedgerInfoWithSignatures) -> Result<(), StateSyncError> {
        fail_point!("consensus::sync_to", |_| {
            Err(anyhow::anyhow!("Injected error in sync_to").into())
        });
        unimplemented!();
    }
}
