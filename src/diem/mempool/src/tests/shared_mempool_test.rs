// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    mocks::MockSharedMempool,
    shared_mempool::types::TransactionSummary,
    tests::common::{batch_add_signed_txn, TestTransaction},
    ConsensusRequest,
};
use diem_types::transaction::Transaction;
use futures::{channel::oneshot, executor::block_on, sink::SinkExt};
use mempool_notifications::MempoolNotificationSender;
use tokio::runtime::Builder;

#[test]
fn test_consensus_events_rejected_txns() {
    let smp = MockSharedMempool::new();

    // Add txns 1, 2, 3, 4
    // Txn 1: committed successfully
    // Txn 2: not committed but older than gc block timestamp
    // Txn 3: not committed and newer than block timestamp
    let committed_txn =
        TestTransaction::new(0, 0, 1).make_signed_transaction_with_expiration_time(0);
    let kept_txn = TestTransaction::new(1, 0, 1).make_signed_transaction(); // not committed or cleaned out by block timestamp gc
    let txns = vec![
        committed_txn.clone(),
        TestTransaction::new(0, 1, 1).make_signed_transaction_with_expiration_time(0),
        kept_txn.clone(),
    ];
    // Add txns to mempool
    {
        let mut pool = smp.mempool.lock();
        assert!(batch_add_signed_txn(&mut pool, txns).is_ok());
    }

    let transactions = vec![TransactionSummary {
        sender: committed_txn.sender(),
        sequence_number: committed_txn.sequence_number(),
    }];
    let (callback, callback_rcv) = oneshot::channel();
    let req = ConsensusRequest::RejectNotification(transactions, callback);
    let mut consensus_sender = smp.consensus_sender.clone();
    block_on(async {
        assert!(consensus_sender.send(req).await.is_ok());
        assert!(callback_rcv.await.is_ok());
    });

    let pool = smp.mempool.lock();
    let (timeline, _) = pool.read_timeline(0, 10);
    assert_eq!(timeline.len(), 1);
    assert_eq!(timeline.get(0).unwrap(), &kept_txn);
}

#[test]
fn test_mempool_notify_committed_txns() {
    // Create runtime for the mempool notifier and listener
    let runtime = Builder::new_multi_thread().enable_all().build().unwrap();
    let _enter = runtime.enter();

    // Create a new mempool notifier, listener and shared mempool
    let smp = MockSharedMempool::new();

    // Add txns 1, 2, 3, 4
    // Txn 1: committed successfully
    // Txn 2: not committed but older than gc block timestamp
    // Txn 3: not committed and newer than block timestamp
    let committed_txn =
        TestTransaction::new(0, 0, 1).make_signed_transaction_with_expiration_time(0);
    let kept_txn = TestTransaction::new(1, 0, 1).make_signed_transaction(); // not committed or cleaned out by block timestamp gc
    let txns = vec![
        committed_txn.clone(),
        TestTransaction::new(0, 1, 1).make_signed_transaction_with_expiration_time(0),
        kept_txn.clone(),
    ];
    // Add txns to mempool
    {
        let mut pool = smp.mempool.lock();
        assert!(batch_add_signed_txn(&mut pool, txns).is_ok());
    }

    let committed_txns = vec![Transaction::UserTransaction(committed_txn)];
    block_on(async {
        assert!(smp
            .mempool_notifier
            .notify_new_commit(committed_txns, 1, 1000)
            .await
            .is_ok());
    });

    let pool = smp.mempool.lock();
    let (timeline, _) = pool.read_timeline(0, 10);
    assert_eq!(timeline.len(), 1);
    assert_eq!(timeline.get(0).unwrap(), &kept_txn);
}
