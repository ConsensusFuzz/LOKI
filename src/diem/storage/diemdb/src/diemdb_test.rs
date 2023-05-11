// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use super::*;
#[allow(unused_imports)]
use crate::{
    schema::jellyfish_merkle_node::JellyfishMerkleNodeSchema,
    test_helper::{arb_blocks_to_commit, arb_mock_genesis},
};
use diem_crypto::hash::CryptoHash;
#[allow(unused_imports)]
use diem_jellyfish_merkle::node_type::{Node, NodeKey};
use diem_temppath::TempPath;
use diem_types::transaction::Transaction;
#[allow(unused_imports)]
use diem_types::{
    account_address::{AccountAddress, HashAccountAddress},
    account_config::AccountResource,
    contract_event::ContractEvent,
    ledger_info::LedgerInfo,
    proof::SparseMerkleLeafNode,
    vm_status::{KeptVMStatus, StatusCode},
};
use proptest::prelude::*;
use std::collections::HashMap;

fn verify_epochs(db: &DiemDB, ledger_infos_with_sigs: &[LedgerInfoWithSignatures]) {
    const LIMIT: usize = 2;
    let mut actual_epoch_change_lis = Vec::new();
    let latest_epoch = ledger_infos_with_sigs
        .last()
        .unwrap()
        .ledger_info()
        .next_block_epoch();

    let mut cursor = 0;
    loop {
        let (chunk, more) = db
            .get_epoch_ending_ledger_infos_impl(cursor, latest_epoch, LIMIT)
            .unwrap();
        actual_epoch_change_lis.extend(chunk);
        if more {
            cursor = actual_epoch_change_lis
                .last()
                .unwrap()
                .ledger_info()
                .next_block_epoch();
        } else {
            break;
        }
    }

    let expected_epoch_change_lis: Vec<_> = ledger_infos_with_sigs
        .iter()
        .filter(|info| info.ledger_info().ends_epoch())
        .cloned()
        .collect();
    assert_eq!(actual_epoch_change_lis, expected_epoch_change_lis);

    let mut last_ver = 0;
    for li in ledger_infos_with_sigs {
        let this_ver = li.ledger_info().version();

        // a version potentially without ledger_info ever committed
        let v1 = (last_ver + this_ver) / 2;
        if v1 != last_ver && v1 != this_ver {
            assert!(db.get_epoch_ending_ledger_info(v1).is_err());
        }

        // a version where there was a ledger_info once
        if li.ledger_info().ends_epoch() {
            assert_eq!(db.get_epoch_ending_ledger_info(this_ver).unwrap(), *li);
        } else {
            assert!(db.get_epoch_ending_ledger_info(this_ver).is_err());
        }
        last_ver = this_ver;
    }
}

pub fn test_save_blocks_impl(input: Vec<(Vec<TransactionToCommit>, LedgerInfoWithSignatures)>) {
    let tmp_dir = TempPath::new();
    let db = DiemDB::new_for_test(&tmp_dir);

    let num_batches = input.len();
    let mut cur_ver = 0;
    let mut all_committed_txns = vec![];
    for (batch_idx, (txns_to_commit, ledger_info_with_sigs)) in input.iter().enumerate() {
        db.save_transactions(
            txns_to_commit,
            cur_ver, /* first_version */
            Some(ledger_info_with_sigs),
        )
        .unwrap();

        assert_eq!(
            db.ledger_store.get_latest_ledger_info().unwrap(),
            *ledger_info_with_sigs
        );
        verify_committed_transactions(
            &db,
            txns_to_commit,
            cur_ver,
            ledger_info_with_sigs,
            batch_idx + 1 == num_batches, /* is_latest */
        );

        // check getting all events by version for all committed transactions
        // up to this point using the current ledger info
        all_committed_txns.extend_from_slice(txns_to_commit);
        verify_get_event_by_version(
            &db,
            &all_committed_txns,
            ledger_info_with_sigs.ledger_info(),
        );

        cur_ver += txns_to_commit.len() as u64;
    }

    let first_batch = input.first().unwrap().0.clone();
    let first_batch_ledger_info = input.first().unwrap().1.clone();
    let latest_ledger_info = input.last().unwrap().1.clone();
    // Verify an old batch with the latest LedgerInfo.
    verify_committed_transactions(
        &db,
        &first_batch,
        0,
        &latest_ledger_info,
        false, /* is_latest */
    );
    // Verify an old batch with an old LedgerInfo.
    verify_committed_transactions(
        &db,
        &first_batch,
        0,
        &first_batch_ledger_info,
        true, /* is_latest */
    );
    let (_, ledger_infos_with_sigs): (Vec<_>, Vec<_>) = input.iter().cloned().unzip();
    verify_epochs(&db, &ledger_infos_with_sigs);
}

fn test_sync_transactions_impl(input: Vec<(Vec<TransactionToCommit>, LedgerInfoWithSignatures)>) {
    let tmp_dir = TempPath::new();
    let db = DiemDB::new_for_test(&tmp_dir);

    let num_batches = input.len();
    let mut cur_ver = 0;
    for (batch_idx, (txns_to_commit, ledger_info_with_sigs)) in input.iter().enumerate() {
        // if batch has more than 2 transactions, save them in two batches
        let batch1_len = txns_to_commit.len() / 2;
        if batch1_len > 0 {
            db.save_transactions(
                &txns_to_commit[..batch1_len],
                cur_ver, /* first_version */
                None,
            )
            .unwrap();
        }
        db.save_transactions(
            &txns_to_commit[batch1_len..],
            cur_ver + batch1_len as u64, /* first_version */
            Some(ledger_info_with_sigs),
        )
        .unwrap();

        verify_committed_transactions(
            &db,
            txns_to_commit,
            cur_ver,
            ledger_info_with_sigs,
            batch_idx + 1 == num_batches, /* is_latest */
        );

        cur_ver += txns_to_commit.len() as u64;
    }
}

fn get_events_by_event_key(
    db: &DiemDB,
    ledger_info: &LedgerInfo,
    event_key: &EventKey,
    first_seq_num: u64,
    last_seq_num: u64,
    order: Order,
    is_latest: bool,
) -> Result<Vec<(Version, ContractEvent)>> {
    const LIMIT: u64 = 3;

    let mut cursor = if order == Order::Ascending {
        first_seq_num
    } else if is_latest {
        // Test the ability to get the latest.
        u64::max_value()
    } else {
        last_seq_num
    };

    let mut ret = Vec::new();
    loop {
        let events_with_proof = db.get_events_with_proof_by_event_key(
            event_key,
            cursor,
            order,
            LIMIT,
            ledger_info.version(),
        )?;

        let num_events = events_with_proof.len() as u64;
        if cursor == u64::max_value() {
            cursor = last_seq_num;
        }
        let expected_seq_nums: Vec<_> = if order == Order::Ascending {
            (cursor..cursor + num_events).collect()
        } else {
            (cursor + 1 - num_events..=cursor).rev().collect()
        };

        let events: Vec<_> = itertools::zip_eq(events_with_proof, expected_seq_nums)
            .map(|(e, seq_num)| {
                e.verify(
                    ledger_info,
                    event_key,
                    seq_num,
                    e.transaction_version,
                    e.event_index,
                )
                .unwrap();
                (e.transaction_version, e.event)
            })
            .collect();

        let num_results = events.len() as u64;
        if num_results == 0 {
            break;
        }
        assert_eq!(events.first().unwrap().1.sequence_number(), cursor);

        if order == Order::Ascending {
            if cursor + num_results > last_seq_num {
                ret.extend(
                    events
                        .into_iter()
                        .take((last_seq_num - cursor + 1) as usize),
                );
                break;
            } else {
                ret.extend(events.into_iter());
                cursor += num_results;
            }
        } else {
            // descending
            if first_seq_num + num_results > cursor {
                ret.extend(
                    events
                        .into_iter()
                        .take((cursor - first_seq_num + 1) as usize),
                );
                break;
            } else {
                ret.extend(events.into_iter());
                cursor -= num_results;
            }
        }
    }

    if order == Order::Descending {
        ret.reverse();
    }

    Ok(ret)
}

fn verify_events_by_event_key(
    db: &DiemDB,
    events: Vec<(EventKey, Vec<(Version, ContractEvent)>)>,
    ledger_info: &LedgerInfo,
    is_latest: bool,
) {
    events
        .into_iter()
        .map(|(access_path, events)| {
            let first_seq = events
                .first()
                .expect("Shouldn't be empty")
                .1
                .sequence_number();
            let last_seq = events
                .last()
                .expect("Shouldn't be empty")
                .1
                .sequence_number();

            let traversed = get_events_by_event_key(
                db,
                ledger_info,
                &access_path,
                first_seq,
                last_seq,
                Order::Ascending,
                is_latest,
            )
            .unwrap();
            assert_eq!(events, traversed);

            let rev_traversed = get_events_by_event_key(
                db,
                ledger_info,
                &access_path,
                first_seq,
                last_seq,
                Order::Descending,
                is_latest,
            )
            .unwrap();
            assert_eq!(events, rev_traversed);
            Ok(())
        })
        .collect::<Result<Vec<_>>>()
        .unwrap();
}

fn group_events_by_event_key(
    first_version: Version,
    txns_to_commit: &[TransactionToCommit],
) -> Vec<(EventKey, Vec<(Version, ContractEvent)>)> {
    let mut event_key_to_events: HashMap<EventKey, Vec<(Version, ContractEvent)>> = HashMap::new();
    for (batch_idx, txn) in txns_to_commit.iter().enumerate() {
        for event in txn.events() {
            event_key_to_events
                .entry(*event.key())
                .or_default()
                .push((first_version + batch_idx as u64, event.clone()));
        }
    }
    event_key_to_events.into_iter().collect()
}

fn verify_get_event_by_version(
    db: &DiemDB,
    committed_txns: &[TransactionToCommit],
    ledger_info: &LedgerInfo,
) {
    let events = group_events_by_event_key(0, committed_txns);

    // just exhaustively check all versions for each set of events
    for (event_key, events) in events {
        for event_version in 0..=ledger_info.version() {
            // find the latest event at or below event_version, or None if
            // event_version < first event.
            let maybe_event_idx = events
                .partition_point(|(txn_version, _event)| *txn_version <= event_version)
                .checked_sub(1);
            let actual = maybe_event_idx.map(|idx| (events[idx].0, &events[idx].1));

            // do the same but via the verifiable DB API
            let event_count = events.len() as u64;
            let event_by_version = db
                .get_event_by_version_with_proof(&event_key, event_version, ledger_info.version())
                .unwrap();
            event_by_version
                .verify(ledger_info, &event_key, Some(event_count), event_version)
                .unwrap();
            // omitting the event count should always pass if we already passed
            // with the actual event count
            event_by_version
                .verify(ledger_info, &event_key, None, event_version)
                .unwrap();
            let expected = event_by_version
                .lower_bound_incl
                .as_ref()
                .map(|proof| (proof.transaction_version, &proof.event));

            // results should be the same
            assert_eq!(actual, expected);

            // quickly check that perturbing a correct proof makes the verification fail
            // TODO(philiphayes): more robust fuzzing?
            let mut bad1 = event_by_version.clone();
            let good = event_by_version;

            std::mem::swap(&mut bad1.lower_bound_incl, &mut bad1.upper_bound_excl);
            if good != bad1 {
                bad1.verify(ledger_info, &event_key, Some(event_count), event_version)
                    .unwrap_err();
            }
        }
    }
}

fn verify_account_txns(
    db: &DiemDB,
    expected_txns_by_account: HashMap<AccountAddress, Vec<(Transaction, Vec<ContractEvent>)>>,
    ledger_info: &LedgerInfo,
) {
    let actual_txns_by_account = expected_txns_by_account
        .iter()
        .map(|(account, txns_and_events)| {
            let account = *account;
            let first_seq_num = if let Some((txn, _)) = txns_and_events.first() {
                txn.as_signed_user_txn().unwrap().sequence_number()
            } else {
                return (account, Vec::new());
            };

            let last_txn = &txns_and_events.last().unwrap().0;
            let last_seq_num = last_txn.as_signed_user_txn().unwrap().sequence_number();
            let limit = last_seq_num + 1;

            let acct_txns_with_proof = db
                .get_account_transactions(
                    account,
                    first_seq_num,
                    limit,
                    true, /* include_events */
                    ledger_info.version(),
                )
                .unwrap();
            acct_txns_with_proof
                .verify(
                    ledger_info,
                    account,
                    first_seq_num,
                    limit,
                    true,
                    ledger_info.version(),
                )
                .unwrap();

            let txns_and_events = acct_txns_with_proof
                .into_inner()
                .into_iter()
                .map(|txn_with_proof| (txn_with_proof.transaction, txn_with_proof.events.unwrap()))
                .collect::<Vec<_>>();

            (account, txns_and_events)
        })
        .collect::<HashMap<_, _>>();

    assert_eq!(actual_txns_by_account, expected_txns_by_account);
}

fn group_txns_by_account(
    txns_to_commit: &[TransactionToCommit],
) -> HashMap<AccountAddress, Vec<(Transaction, Vec<ContractEvent>)>> {
    let mut account_to_txns = HashMap::new();
    for txn in txns_to_commit {
        if let Ok(signed_txn) = txn.transaction().as_signed_user_txn() {
            let account = signed_txn.sender();
            account_to_txns
                .entry(account)
                .or_insert_with(Vec::new)
                .push((txn.transaction().clone(), txn.events().to_vec()));
        }
    }
    account_to_txns
}

fn verify_committed_transactions(
    db: &DiemDB,
    txns_to_commit: &[TransactionToCommit],
    first_version: Version,
    ledger_info_with_sigs: &LedgerInfoWithSignatures,
    is_latest: bool,
) {
    let ledger_info = ledger_info_with_sigs.ledger_info();
    let ledger_version = ledger_info.version();
    assert_eq!(
        db.get_accumulator_root_hash(ledger_version).unwrap(),
        ledger_info.transaction_accumulator_hash()
    );

    let mut cur_ver = first_version;
    for txn_to_commit in txns_to_commit {
        let txn_info = db.ledger_store.get_transaction_info(cur_ver).unwrap();

        // Verify transaction hash.
        assert_eq!(
            txn_info.transaction_hash(),
            txn_to_commit.transaction().hash()
        );

        // Fetch and verify transaction itself.
        let txn = txn_to_commit.transaction().as_signed_user_txn().unwrap();
        let txn_with_proof = db
            .get_transaction_by_hash(txn_to_commit.transaction().hash(), ledger_version, true)
            .unwrap()
            .unwrap();
        assert_eq!(
            txn_with_proof.transaction.hash(),
            txn_to_commit.transaction().hash()
        );
        txn_with_proof
            .verify_user_txn(ledger_info, cur_ver, txn.sender(), txn.sequence_number())
            .unwrap();
        let txn_with_proof = db
            .get_transaction_with_proof(cur_ver, ledger_version, true)
            .unwrap();
        txn_with_proof
            .verify_user_txn(ledger_info, cur_ver, txn.sender(), txn.sequence_number())
            .unwrap();

        let txn_with_proof = db
            .get_account_transaction(txn.sender(), txn.sequence_number(), true, ledger_version)
            .unwrap()
            .expect("Should exist.");
        txn_with_proof
            .verify_user_txn(ledger_info, cur_ver, txn.sender(), txn.sequence_number())
            .unwrap();

        let acct_txns_with_proof = db
            .get_account_transactions(txn.sender(), txn.sequence_number(), 1, true, ledger_version)
            .unwrap();
        acct_txns_with_proof
            .verify(
                ledger_info,
                txn.sender(),
                txn.sequence_number(),
                1,
                true,
                ledger_version,
            )
            .unwrap();
        assert_eq!(acct_txns_with_proof.len(), 1);

        let txn_list_with_proof = db
            .get_transactions(cur_ver, 1, ledger_version, true /* fetch_events */)
            .unwrap();
        txn_list_with_proof
            .verify(ledger_info, Some(cur_ver))
            .unwrap();
        assert_eq!(txn_list_with_proof.transactions.len(), 1);

        let txn_output_list_with_proof = db
            .get_transaction_outputs(cur_ver, 1, ledger_version)
            .unwrap();
        txn_output_list_with_proof
            .verify(ledger_info, Some(cur_ver))
            .unwrap();
        assert_eq!(txn_output_list_with_proof.transactions_and_outputs.len(), 1);

        // Fetch and verify account states.
        for (addr, expected_blob) in txn_to_commit.account_states() {
            let account_state_with_proof = db
                .get_account_state_with_proof(*addr, cur_ver, ledger_version)
                .unwrap();
            assert_eq!(account_state_with_proof.blob, Some(expected_blob.clone()));
            account_state_with_proof
                .verify(ledger_info, cur_ver, *addr)
                .unwrap();
        }

        cur_ver += 1;
    }

    // Fetch and verify events.
    verify_events_by_event_key(
        db,
        group_events_by_event_key(first_version, txns_to_commit),
        ledger_info,
        is_latest,
    );

    // Fetch and verify batch transactions by account
    verify_account_txns(db, group_txns_by_account(txns_to_commit), ledger_info);
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(10))]

    #[test]
    fn test_save_blocks(input in arb_blocks_to_commit()) {
        test_save_blocks_impl(input);
    }

    #[test]
    fn test_sync_transactions(input in arb_blocks_to_commit()) {
        test_sync_transactions_impl(input);
    }
}

#[test]
fn test_get_first_seq_num_and_limit() {
    assert!(get_first_seq_num_and_limit(Order::Ascending, 0, 0).is_err());

    // ascending
    assert_eq!(
        get_first_seq_num_and_limit(Order::Ascending, 0, 4).unwrap(),
        (0, 4)
    );
    assert_eq!(
        get_first_seq_num_and_limit(Order::Ascending, 0, 1).unwrap(),
        (0, 1)
    );

    // descending
    assert_eq!(
        get_first_seq_num_and_limit(Order::Descending, 2, 1).unwrap(),
        (2, 1)
    );
    assert_eq!(
        get_first_seq_num_and_limit(Order::Descending, 2, 2).unwrap(),
        (1, 2)
    );
    assert_eq!(
        get_first_seq_num_and_limit(Order::Descending, 2, 3).unwrap(),
        (0, 3)
    );
    assert_eq!(
        get_first_seq_num_and_limit(Order::Descending, 2, 4).unwrap(),
        (0, 3)
    );
}

#[test]
fn test_too_many_requested() {
    let tmp_dir = TempPath::new();
    let db = DiemDB::new_for_test(&tmp_dir);

    assert!(db.get_transactions(0, 1001 /* limit */, 0, true).is_err());
    assert!(db.get_transaction_outputs(0, 1001 /* limit */, 0).is_err());
}

#[test]
fn test_get_latest_tree_state() {
    let tmp_dir = TempPath::new();
    let db = DiemDB::new_for_test(&tmp_dir);

    // entirely emtpy db
    let empty = db.get_latest_tree_state().unwrap();
    assert_eq!(
        empty,
        TreeState::new(0, vec![], *SPARSE_MERKLE_PLACEHOLDER_HASH,)
    );

    // unbootstrapped db with pre-genesis state
    let address = AccountAddress::ZERO;
    let blob = AccountStateBlob::from(vec![1]);
    db.db
        .put::<JellyfishMerkleNodeSchema>(
            &NodeKey::new_empty_path(PRE_GENESIS_VERSION),
            &Node::new_leaf(address.hash(), blob.clone()),
        )
        .unwrap();
    let hash = SparseMerkleLeafNode::new(address.hash(), blob.hash()).hash();
    let pre_genesis = db.get_latest_tree_state().unwrap();
    assert_eq!(pre_genesis, TreeState::new(0, vec![], hash));

    // bootstrapped db (any transaction info is in)
    let txn_info = TransactionInfo::new(
        HashValue::random(),
        HashValue::random(),
        HashValue::random(),
        0,
        KeptVMStatus::MiscellaneousError,
    );
    put_transaction_info(&db, 0, &txn_info);
    let bootstrapped = db.get_latest_tree_state().unwrap();
    assert_eq!(
        bootstrapped,
        TreeState::new(1, vec![txn_info.hash()], txn_info.state_root_hash())
    );
}

fn put_transaction_info(db: &DiemDB, version: Version, txn_info: &TransactionInfo) {
    let mut cs = ChangeSet::new();
    db.ledger_store
        .put_transaction_infos(version, &[txn_info.clone()], &mut cs)
        .unwrap();
    db.db.write_schemas(cs.batch).unwrap();
}
