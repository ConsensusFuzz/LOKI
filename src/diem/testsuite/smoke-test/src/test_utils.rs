// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use diem_config::config::{Identity, NodeConfig, SecureBackend};
use diem_crypto::ed25519::Ed25519PublicKey;
use diem_sdk::{
    client::BlockingClient,
    transaction_builder::{Currency, TransactionFactory},
    types::{transaction::SignedTransaction, LocalAccount},
};
use forge::{LocalSwarm, NodeExt, Swarm};
use std::{fs::File, io::Write, path::PathBuf};

pub fn create_and_fund_account(swarm: &mut LocalSwarm, amount: u64) -> LocalAccount {
    let account = LocalAccount::generate(&mut rand::rngs::OsRng);
    swarm
        .chain_info()
        .create_parent_vasp_account(Currency::XUS, account.authentication_key())
        .unwrap();
    swarm
        .chain_info()
        .fund(Currency::XUS, account.address(), amount)
        .unwrap();
    account
}

pub fn transfer_coins_non_blocking(
    client: &BlockingClient,
    transaction_factory: &TransactionFactory,
    sender: &mut LocalAccount,
    receiver: &LocalAccount,
    amount: u64,
) -> SignedTransaction {
    let txn = sender.sign_with_transaction_builder(transaction_factory.peer_to_peer(
        Currency::XUS,
        receiver.address(),
        amount,
    ));

    client.submit(&txn).unwrap();
    txn
}

pub fn transfer_coins(
    client: &BlockingClient,
    transaction_factory: &TransactionFactory,
    sender: &mut LocalAccount,
    receiver: &LocalAccount,
    amount: u64,
) -> SignedTransaction {
    let txn = transfer_coins_non_blocking(client, transaction_factory, sender, receiver, amount);

    client
        .wait_for_signed_transaction(&txn, None, None)
        .unwrap();

    txn
}

pub fn assert_balance(client: &BlockingClient, account: &LocalAccount, balance: u64) {
    let account_view = client
        .get_account(account.address())
        .unwrap()
        .into_inner()
        .unwrap();

    let onchain_balance = account_view
        .balances
        .into_iter()
        .find(|amount_view| amount_view.currency == Currency::XUS)
        .unwrap();
    assert_eq!(onchain_balance.amount, balance);
}

/// This module provides useful functions for operating, handling and managing
/// DiemSwarm instances. It is particularly useful for working with tests that
/// require a SmokeTestEnvironment, as it provides a generic interface across
/// DiemSwarms, regardless of if the swarm is a validator swarm, validator full
/// node swarm, or a public full node swarm.
pub mod diem_swarm_utils {
    use crate::test_utils::fetch_backend_storage;
    use diem_config::config::{NodeConfig, OnDiskStorageConfig, SecureBackend, WaypointConfig};
    use diem_global_constants::{DIEM_ROOT_KEY, TREASURY_COMPLIANCE_KEY};
    use diem_secure_storage::{CryptoStorage, KVStorage, OnDiskStorage, Storage};
    use diem_types::waypoint::Waypoint;
    use forge::{LocalNode, LocalSwarm, Swarm};

    /// Loads the nodes's storage backend identified by the node index in the given swarm.
    pub fn load_validators_backend_storage(validator: &LocalNode) -> SecureBackend {
        fetch_backend_storage(validator.config(), None)
    }

    pub fn create_root_storage(swarm: &mut LocalSwarm) -> SecureBackend {
        let chain_info = swarm.chain_info();
        let root_key =
            bcs::from_bytes(&bcs::to_bytes(chain_info.root_account.private_key()).unwrap())
                .unwrap();
        let treasury_compliance_key = bcs::from_bytes(
            &bcs::to_bytes(chain_info.treasury_compliance_account.private_key()).unwrap(),
        )
        .unwrap();

        let mut root_storage_config = OnDiskStorageConfig::default();
        root_storage_config.path = swarm.dir().join("root-storage.json");
        let mut root_storage = OnDiskStorage::new(root_storage_config.path());
        root_storage
            .import_private_key(DIEM_ROOT_KEY, root_key)
            .unwrap();
        root_storage
            .import_private_key(TREASURY_COMPLIANCE_KEY, treasury_compliance_key)
            .unwrap();

        SecureBackend::OnDiskStorage(root_storage_config)
    }

    pub fn insert_waypoint(node_config: &mut NodeConfig, waypoint: Waypoint) {
        let f = |backend: &SecureBackend| {
            let mut storage: Storage = backend.into();
            storage
                .set(diem_global_constants::WAYPOINT, waypoint)
                .expect("Unable to write waypoint");
            storage
                .set(diem_global_constants::GENESIS_WAYPOINT, waypoint)
                .expect("Unable to write waypoint");
        };
        let backend = &node_config.consensus.safety_rules.backend;
        f(backend);
        match &node_config.base.waypoint {
            WaypointConfig::FromStorage(backend) => {
                f(backend);
            }
            _ => panic!("unexpected waypoint from node config"),
        }
    }
}

/// Loads the node's storage backend from the given node config. If a namespace
/// is specified, the storage namespace will be overridden.
fn fetch_backend_storage(
    node_config: &NodeConfig,
    overriding_namespace: Option<String>,
) -> SecureBackend {
    if let Identity::FromStorage(storage_identity) =
        &node_config.validator_network.as_ref().unwrap().identity
    {
        match storage_identity.backend.clone() {
            SecureBackend::OnDiskStorage(mut config) => {
                if let Some(namespace) = overriding_namespace {
                    config.namespace = Some(namespace);
                }
                SecureBackend::OnDiskStorage(config)
            }
            _ => unimplemented!("On-disk storage is the only backend supported in smoke tests"),
        }
    } else {
        panic!("Couldn't load identity from storage");
    }
}

/// Writes a given public key to a file specified by the given path using hex encoding.
/// Contents are written using utf-8 encoding and a newline is appended to ensure that
/// whitespace can be handled by tests.
pub fn write_key_to_file_hex_format(key: &Ed25519PublicKey, key_file_path: PathBuf) {
    let hex_encoded_key = hex::encode(key.to_bytes());
    let key_and_newline = hex_encoded_key + "\n";
    let mut file = File::create(key_file_path).unwrap();
    file.write_all(key_and_newline.as_bytes()).unwrap();
}

/// Writes a given public key to a file specified by the given path using bcs encoding.
pub fn write_key_to_file_bcs_format(key: &Ed25519PublicKey, key_file_path: PathBuf) {
    let bcs_encoded_key = bcs::to_bytes(&key).unwrap();
    let mut file = File::create(key_file_path).unwrap();
    file.write_all(&bcs_encoded_key).unwrap();
}

/// This helper function creates 3 new accounts, mints funds, transfers funds
/// between the accounts and verifies that these operations succeed.
pub fn check_create_mint_transfer(swarm: &mut LocalSwarm) {
    let client = swarm.validators().next().unwrap().json_rpc_client();
    let transaction_factory = swarm.chain_info().transaction_factory();

    // Create account 0, mint 10 coins and check balance
    let mut account_0 = create_and_fund_account(swarm, 10);
    assert_balance(&client, &account_0, 10);

    // Create account 1, mint 1 coin, transfer 3 coins from account 0 to 1, check balances
    let account_1 = create_and_fund_account(swarm, 1);
    transfer_coins(&client, &transaction_factory, &mut account_0, &account_1, 3);

    assert_balance(&client, &account_0, 7);
    assert_balance(&client, &account_1, 4);

    // Create account 2, mint 15 coins and check balance
    let account_2 = create_and_fund_account(swarm, 15);
    assert_balance(&client, &account_2, 15);
}
