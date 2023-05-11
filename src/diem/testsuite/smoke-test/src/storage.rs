// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    smoke_test_environment::new_local_swarm,
    test_utils::{
        assert_balance, create_and_fund_account, diem_swarm_utils::insert_waypoint, transfer_coins,
    },
    workspace_builder,
    workspace_builder::workspace_root,
};
use anyhow::{bail, Result};
use backup_cli::metadata::view::BackupStorageState;
use diem_sdk::{
    client::BlockingClient, transaction_builder::TransactionFactory, types::LocalAccount,
};
use diem_temppath::TempPath;
use diem_types::{transaction::Version, waypoint::Waypoint};
use forge::{NodeExt, Swarm, SwarmExt};
use rand::random;
use std::{
    fs,
    path::Path,
    process::Command,
    time::{Duration, Instant},
};

#[test]
fn test_db_restore() {
    // pre-build tools
    workspace_builder::get_bin("db-backup");
    workspace_builder::get_bin("db-restore");
    workspace_builder::get_bin("db-backup-verify");

    let mut swarm = new_local_swarm(4);
    let validator_peer_ids = swarm.validators().map(|v| v.peer_id()).collect::<Vec<_>>();
    let client_1 = swarm
        .validator(validator_peer_ids[1])
        .unwrap()
        .json_rpc_client();
    let transaction_factory = swarm.chain_info().transaction_factory();

    // set up: two accounts, a lot of money
    let mut account_0 = create_and_fund_account(&mut swarm, 1000000);
    let account_1 = create_and_fund_account(&mut swarm, 1000000);
    transfer_coins(
        &client_1,
        &transaction_factory,
        &mut account_0,
        &account_1,
        1,
    );

    let mut expected_balance_0 = 999999;
    let mut expected_balance_1 = 1000001;
    assert_balance(&client_1, &account_0, expected_balance_0);
    assert_balance(&client_1, &account_1, expected_balance_1);

    transfer_and_reconfig(
        &client_1,
        &transaction_factory,
        swarm.chain_info().root_account,
        &mut account_0,
        &account_1,
        20,
    )
    .unwrap();
    expected_balance_0 -= 20;
    expected_balance_1 += 20;
    assert_balance(&client_1, &account_0, expected_balance_0);
    assert_balance(&client_1, &account_1, expected_balance_1);

    // make a backup from node 1
    let node1_config = swarm.validator(validator_peer_ids[1]).unwrap().config();
    let backup_path = db_backup(
        node1_config.storage.backup_service_address.port(),
        1,
        50,
        20,
        40,
        &[],
    );

    // take down node 0
    let node_to_restart = validator_peer_ids[0];
    swarm.validator_mut(node_to_restart).unwrap().stop();

    // nuke db
    let node0_config_path = swarm.validator(node_to_restart).unwrap().config_path();
    let mut node0_config = swarm.validator(node_to_restart).unwrap().config().clone();
    let genesis_waypoint = node0_config.base.waypoint.genesis_waypoint();
    insert_waypoint(&mut node0_config, genesis_waypoint);
    node0_config.save(node0_config_path).unwrap();
    let db_dir = node0_config.storage.dir();
    fs::remove_dir_all(db_dir.join("diemdb")).unwrap();
    fs::remove_dir_all(db_dir.join("consensusdb")).unwrap();

    // restore db from backup
    db_restore(backup_path.path(), db_dir.as_path(), &[]);

    {
        transfer_and_reconfig(
            &client_1,
            &transaction_factory,
            swarm.chain_info().root_account,
            &mut account_0,
            &account_1,
            20,
        )
        .unwrap();
        expected_balance_0 -= 20;
        expected_balance_1 += 20;
        assert_balance(&client_1, &account_0, expected_balance_0);
        assert_balance(&client_1, &account_1, expected_balance_1);
    }

    // start node 0 on top of restored db
    swarm
        .validator_mut(node_to_restart)
        .unwrap()
        .start()
        .unwrap();
    swarm
        .validator_mut(node_to_restart)
        .unwrap()
        .wait_until_healthy(Instant::now() + Duration::from_secs(10))
        .unwrap();
    // verify it's caught up
    swarm
        .wait_for_all_nodes_to_catchup(Instant::now() + Duration::from_secs(60))
        .unwrap();

    let client_0 = swarm.validator(node_to_restart).unwrap().json_rpc_client();
    assert_balance(&client_0, &account_0, expected_balance_0);
    assert_balance(&client_0, &account_1, expected_balance_1);
}

fn db_backup_verify(backup_path: &Path, trusted_waypoints: &[Waypoint]) {
    let now = Instant::now();
    let bin_path = workspace_builder::get_bin("db-backup-verify");
    let metadata_cache_path = TempPath::new();

    metadata_cache_path.create_as_dir().unwrap();

    let mut cmd = Command::new(bin_path.as_path());

    trusted_waypoints.iter().for_each(|w| {
        cmd.arg("--trust-waypoint");
        cmd.arg(&w.to_string());
    });

    let output = cmd
        .args(&[
            "--metadata-cache-dir",
            metadata_cache_path.path().to_str().unwrap(),
            "local-fs",
            "--dir",
            backup_path.to_str().unwrap(),
        ])
        .current_dir(workspace_root())
        .output()
        .unwrap();
    if !output.status.success() {
        panic!("db-backup-verify failed, output: {:?}", output);
    }
    println!("Backup verified in {} seconds.", now.elapsed().as_secs());
}

fn wait_for_backups(
    target_epoch: u64,
    target_version: u64,
    now: Instant,
    bin_path: &Path,
    metadata_cache_path: &Path,
    backup_path: &Path,
    trusted_waypoints: &[Waypoint],
) -> Result<()> {
    for _ in 0..60 {
        // the verify should always succeed.
        db_backup_verify(backup_path, trusted_waypoints);

        let output = Command::new(bin_path)
            .current_dir(workspace_root())
            .args(&[
                "one-shot",
                "query",
                "backup-storage-state",
                "--metadata-cache-dir",
                metadata_cache_path.to_str().unwrap(),
                "local-fs",
                "--dir",
                backup_path.to_str().unwrap(),
            ])
            .output()?
            .stdout;
        let state: BackupStorageState = std::str::from_utf8(&output)?.parse()?;
        if state.latest_epoch_ending_epoch.is_some()
            && state.latest_transaction_version.is_some()
            && state.latest_state_snapshot_version.is_some()
            && state.latest_epoch_ending_epoch.unwrap() >= target_epoch
            && state.latest_transaction_version.unwrap() >= target_version
        {
            println!("Backup created in {} seconds.", now.elapsed().as_secs());
            return Ok(());
        }
        println!("Backup storage state: {}", state);
        std::thread::sleep(Duration::from_secs(1));
    }

    bail!("Failed to create backup.");
}

pub(crate) fn db_backup(
    backup_service_port: u16,
    target_epoch: u64,
    target_version: Version,
    transaction_batch_size: usize,
    state_snapshot_interval: usize,
    trusted_waypoints: &[Waypoint],
) -> TempPath {
    let now = Instant::now();
    let bin_path = workspace_builder::get_bin("db-backup");
    let metadata_cache_path1 = TempPath::new();
    let metadata_cache_path2 = TempPath::new();
    let backup_path = TempPath::new();

    metadata_cache_path1.create_as_dir().unwrap();
    metadata_cache_path2.create_as_dir().unwrap();
    backup_path.create_as_dir().unwrap();

    // spawn the backup coordinator
    let mut backup_coordinator = Command::new(bin_path.as_path())
        .current_dir(workspace_root())
        .args(&[
            "coordinator",
            "run",
            "--backup-service-address",
            &format!("http://localhost:{}", backup_service_port),
            "--transaction-batch-size",
            &transaction_batch_size.to_string(),
            "--state-snapshot-interval",
            &state_snapshot_interval.to_string(),
            "--metadata-cache-dir",
            metadata_cache_path1.path().to_str().unwrap(),
            "local-fs",
            "--dir",
            backup_path.path().to_str().unwrap(),
        ])
        .spawn()
        .unwrap();

    // watch the backup storage, wait for it to reach target epoch and version
    let wait_res = wait_for_backups(
        target_epoch,
        target_version,
        now,
        bin_path.as_path(),
        metadata_cache_path2.path(),
        backup_path.path(),
        trusted_waypoints,
    );
    backup_coordinator.kill().unwrap();
    wait_res.unwrap();
    backup_path
}

pub(crate) fn db_restore(backup_path: &Path, db_path: &Path, trusted_waypoints: &[Waypoint]) {
    let now = Instant::now();
    let bin_path = workspace_builder::get_bin("db-restore");
    let metadata_cache_path = TempPath::new();

    metadata_cache_path.create_as_dir().unwrap();

    let mut cmd = Command::new(bin_path.as_path());
    trusted_waypoints.iter().for_each(|w| {
        cmd.arg("--trust-waypoint");
        cmd.arg(&w.to_string());
    });

    let output = cmd
        .args(&[
            "--target-db-dir",
            db_path.to_str().unwrap(),
            "auto",
            "--metadata-cache-dir",
            metadata_cache_path.path().to_str().unwrap(),
            "local-fs",
            "--dir",
            backup_path.to_str().unwrap(),
        ])
        .current_dir(workspace_root())
        .output()
        .unwrap();
    if !output.status.success() {
        panic!("db-restore failed, output: {:?}", output);
    }
    println!("Backup restored in {} seconds.", now.elapsed().as_secs());
}

fn transfer_and_reconfig(
    client: &BlockingClient,
    transaction_factory: &TransactionFactory,
    root_account: &mut LocalAccount,
    account0: &mut LocalAccount,
    account1: &LocalAccount,
    transfers: usize,
) -> Result<()> {
    for _ in 0..transfers {
        if random::<u16>() % 10 == 0 {
            let current_version = client.get_metadata()?.into_inner().diem_version.unwrap();
            let txn = root_account.sign_with_transaction_builder(
                transaction_factory.update_diem_version(0, current_version + 1),
            );
            client.submit(&txn)?;
            client.wait_for_signed_transaction(&txn, None, None)?;

            println!("Changing diem version to {}", current_version + 1,);
        }

        transfer_coins(client, transaction_factory, account0, account1, 1);
    }

    Ok(())
}
