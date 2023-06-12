// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{corpus_from_strategy, fuzz_data_to_value, FuzzTargetImpl};
use diem_proptest_helpers::ValueGenerator;
use state_sync::fuzzing::{arb_state_sync_msg, test_state_sync_msg_fuzzer_impl};

#[derive(Debug, Default)]
pub struct StateSyncMsg;

impl FuzzTargetImpl for StateSyncMsg {
    fn description(&self) -> &'static str {
        "State sync network message"
    }

    fn generate(&self, _idx: usize, _gen: &mut ValueGenerator) -> Option<Vec<u8>> {
        Some(corpus_from_strategy(arb_state_sync_msg()))
    }

    fn fuzz(&self, data: &[u8]) {
        let msg = fuzz_data_to_value(data, arb_state_sync_msg());
        test_state_sync_msg_fuzzer_impl(msg);
    }
}
