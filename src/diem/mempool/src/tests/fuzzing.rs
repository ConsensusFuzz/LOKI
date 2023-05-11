// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    core_mempool::{CoreMempool, TimelineState},
    shared_mempool::{tasks, types::SharedMempool},
};
use diem_config::{config::NodeConfig, network_id::NetworkId};
use diem_infallible::{Mutex, RwLock};
use diem_types::transaction::SignedTransaction;
use network::application::storage::PeerMetadataStorage;
use proptest::{
    arbitrary::any,
    prelude::*,
    strategy::{Just, Strategy},
};
use std::{collections::HashMap, sync::Arc};
use storage_interface::mock::MockDbReaderWriter;
use vm_validator::mocks::mock_vm_validator::MockVMValidator;

pub fn mempool_incoming_transactions_strategy(
) -> impl Strategy<Value = (Vec<SignedTransaction>, TimelineState)> {
    (
        proptest::collection::vec(any::<SignedTransaction>(), 0..100),
        prop_oneof![
            Just(TimelineState::NotReady),
            Just(TimelineState::NonQualified)
        ],
    )
}

pub fn test_mempool_process_incoming_transactions_impl(
    txns: Vec<SignedTransaction>,
    timeline_state: TimelineState,
) {
    let config = NodeConfig::default();
    let mock_db = MockDbReaderWriter;
    let vm_validator = Arc::new(RwLock::new(MockVMValidator));
    let smp = SharedMempool::new(
        Arc::new(Mutex::new(CoreMempool::new(&config))),
        config.mempool.clone(),
        HashMap::new(),
        Arc::new(mock_db),
        vm_validator,
        vec![],
        config.base.role,
        PeerMetadataStorage::new(&[NetworkId::Validator]),
    );

    let _ = tasks::process_incoming_transactions(&smp, txns, timeline_state);
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(10))]

    #[test]
    fn test_mempool_process_incoming_transactions((txns, timeline_state) in mempool_incoming_transactions_strategy()) {
        test_mempool_process_incoming_transactions_impl(txns, timeline_state);
    }
}
