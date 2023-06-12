// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::experimental::{
    execution_phase::ExecutionPhase, tests::ordering_state_computer_tests::random_empty_block,
};

use diem_types::ledger_info::{LedgerInfo, LedgerInfoWithSignatures};

use crate::{
    experimental::{commit_phase::CommitChannelType, execution_phase::ExecutionChannelType},
    state_replication::empty_state_computer_call_back,
    test_utils::{consensus_runtime, timed_block_on, RandomComputeResultStateComputer},
};
use consensus_types::block::block_test_utils::certificate_for_genesis;
use diem_crypto::{ed25519::Ed25519Signature, hash::ACCUMULATOR_PLACEHOLDER_HASH};
use diem_types::{account_address::AccountAddress, validator_verifier::random_validator_verifier};
use executor_types::StateComputeResult;
use futures::{SinkExt, StreamExt};
use std::{collections::BTreeMap, sync::Arc};

#[test]
fn test_execution_phase() {
    let num_nodes = 1;
    let channel_size = 30;
    let mut runtime = consensus_runtime();

    let (mut execution_phase_tx, execution_phase_rx) =
        channel::new_test::<ExecutionChannelType>(channel_size);

    let (commit_phase_tx, mut commit_phase_rx) =
        channel::new_test::<CommitChannelType>(channel_size);

    let random_state_computer = RandomComputeResultStateComputer::new();
    let random_execute_result_root_hash = random_state_computer.get_root_hash();

    let execution_phase = ExecutionPhase::new(
        execution_phase_rx,
        Arc::new(random_state_computer),
        commit_phase_tx,
    );

    runtime.spawn(execution_phase.start());

    let (signers, _) = random_validator_verifier(num_nodes, None, false);
    let signer = &signers[0];
    let genesis_qc = certificate_for_genesis();
    let block = random_empty_block(signer, genesis_qc);

    let dummy_state_compute_result = StateComputeResult::new_dummy();

    let li = LedgerInfo::new(
        block.gen_block_info(
            dummy_state_compute_result.root_hash(),
            dummy_state_compute_result.version(),
            dummy_state_compute_result.epoch_state().clone(),
        ),
        *ACCUMULATOR_PLACEHOLDER_HASH,
    );

    let li_sig =
        LedgerInfoWithSignatures::new(li, BTreeMap::<AccountAddress, Ed25519Signature>::new());

    let blocks = vec![block.clone()];

    timed_block_on(&mut runtime, async move {
        execution_phase_tx
            .send((blocks, li_sig.clone(), empty_state_computer_call_back()))
            .await
            .ok();
        let (executed_blocks, executed_finality_proof, _) = commit_phase_rx.next().await.unwrap();
        assert_eq!(executed_blocks.len(), 1);
        assert_eq!(
            executed_blocks[0].compute_result(),
            &StateComputeResult::new_dummy_with_root_hash(random_execute_result_root_hash)
        );
        assert_eq!(executed_blocks[0].block(), &block);
        assert_eq!(executed_finality_proof, li_sig);
    });
}
