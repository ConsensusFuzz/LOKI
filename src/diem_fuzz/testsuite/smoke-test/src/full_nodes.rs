// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{smoke_test_environment::SmokeTestEnvironment, test_utils::compare_balances};
use cli::client_proxy::{ClientProxy, IndexAndSequence};
use diem_config::{
    config::{DiscoveryMethod, NodeConfig, Peer, PeerRole, HANDSHAKE_VERSION},
    network_id::NetworkId,
};
use diem_sdk::client::AccountAddress;
use diem_types::{
    account_config::{testnet_dd_account_address, treasury_compliance_account_address},
    network_address::{NetworkAddress, Protocol},
};
use std::{collections::HashSet, net::Ipv4Addr};

// TODO: All of this convenience code below should be put in the client proxy directly, it's
// very hard for users to know what the order of a bunch of random inputs should be
fn get_and_reset_sequence_number(client: &mut ClientProxy, account: AccountAddress) -> u64 {
    sequence_number_inner(client, account, true)
}

fn sequence_number_inner(
    client: &mut ClientProxy,
    account: AccountAddress,
    reset_sequence_number: bool,
) -> u64 {
    let command_string = format!("sequence {} {}", account, reset_sequence_number);
    let command: Vec<_> = command_string.split(' ').collect();
    client.get_sequence_number(&command).unwrap()
}

fn mint_coins(
    client: &mut ClientProxy,
    account_num: usize,
    amount: u64,
    currency: &str,
    is_blocking: bool,
) {
    client
        .mint_coins(
            &[
                "mintb",
                &account_num.to_string(),
                &amount.to_string(),
                currency,
            ],
            is_blocking,
        )
        .expect("Failed to mint coins!")
}

fn transfer_coins(
    client: &mut ClientProxy,
    sender_address_num: usize,
    receiver_address_num: usize,
    amount: u64,
    currency: &str,
    is_blocking: bool,
) -> IndexAndSequence {
    client
        .transfer_coins(
            &[
                "tb",
                &sender_address_num.to_string(),
                &receiver_address_num.to_string(),
                &amount.to_string(),
                currency,
            ],
            is_blocking,
        )
        .expect("Failed to transfer coins!")
}

fn get_balances(client: &mut ClientProxy, account_num: usize) -> Vec<String> {
    let account = account_num.to_string();
    client.get_balances(&["b", &account]).unwrap()
}

fn wait_for_transaction(client: &mut ClientProxy, account: AccountAddress, seq_num: u64) {
    client
        .wait_for_transaction(account, seq_num)
        .expect("Expected transaction")
}

const XUS: &str = "XUS";
const PUBLIC: &str = "public";

#[test]
fn test_full_node_basic_flow() {
    let mut env = SmokeTestEnvironment::new(7);
    env.setup_vfn_swarm();
    let vfn_swarm = env.vfn_swarm();
    env.add_public_fn_swarm(PUBLIC, 2, None, &vfn_swarm.lock().config);

    env.validator_swarm.launch();
    env.vfn_swarm().lock().launch();
    env.public_swarm(PUBLIC).lock().launch();

    // read state from full node client
    let mut validator_client = env.get_validator_client(0, None);
    let mut vfn_client = env.get_vfn_client(1, None);
    let mut pfn_client = env.get_pfn_client(PUBLIC, 0, None);

    // mint from full node and check both validator and full node have correct balance
    let account = validator_client.create_next_account(false).unwrap().address;
    vfn_client.create_next_account(false).unwrap();
    pfn_client.create_next_account(false).unwrap();

    let sender_account = testnet_dd_account_address();
    let creation_account = treasury_compliance_account_address();

    get_and_reset_sequence_number(&mut vfn_client, sender_account);
    get_and_reset_sequence_number(&mut vfn_client, creation_account);
    mint_coins(&mut vfn_client, 0, 10, XUS, true);

    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut vfn_client, 0)
    ));

    let sequence = get_and_reset_sequence_number(&mut vfn_client, sender_account);
    wait_for_transaction(&mut validator_client, sender_account, sequence - 1);
    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut validator_client, 0),
    ));

    // reset sequence number for sender account
    get_and_reset_sequence_number(&mut validator_client, sender_account);
    get_and_reset_sequence_number(&mut vfn_client, sender_account);
    get_and_reset_sequence_number(&mut validator_client, creation_account);
    get_and_reset_sequence_number(&mut vfn_client, creation_account);

    // mint from validator and check both nodes have correct balance
    validator_client.create_next_account(false).unwrap();
    vfn_client.create_next_account(false).unwrap();
    pfn_client.create_next_account(false).unwrap();

    mint_coins(&mut validator_client, 1, 10, XUS, true);
    let sequence = get_and_reset_sequence_number(&mut validator_client, sender_account);
    wait_for_transaction(&mut vfn_client, sender_account, sequence - 1);

    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut validator_client, 1)
    ));
    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut vfn_client, 1)
    ));

    // minting again on validator doesn't cause error since client sequence has been updated
    mint_coins(&mut validator_client, 1, 10, XUS, true);

    // test transferring balance from 0 to 1 through full node proxy
    transfer_coins(&mut vfn_client, 0, 1, 10, XUS, true);

    assert!(compare_balances(
        vec![(0.0, XUS.to_string())],
        get_balances(&mut vfn_client, 0)
    ));
    assert!(compare_balances(
        vec![(30.0, XUS.to_string())],
        get_balances(&mut validator_client, 1)
    ));

    let sequence = get_and_reset_sequence_number(&mut validator_client, account);
    wait_for_transaction(&mut pfn_client, account, sequence - 1);
    assert!(compare_balances(
        vec![(0.0, XUS.to_string())],
        get_balances(&mut pfn_client, 0)
    ));
    assert!(compare_balances(
        vec![(30.0, XUS.to_string())],
        get_balances(&mut pfn_client, 1)
    ));
}

#[test]
fn test_vfn_failover() {
    let mut env = SmokeTestEnvironment::new(7);
    env.setup_vfn_swarm();
    let vfn_swarm = env.vfn_swarm();
    env.add_public_fn_swarm(PUBLIC, 1, None, &vfn_swarm.lock().config);

    env.validator_swarm.launch();
    env.vfn_swarm().lock().launch();
    env.public_swarm(PUBLIC).lock().launch();

    // set up clients
    let mut vfn_0_client = env.get_vfn_client(0, None);
    let mut vfn_1_client = env.get_vfn_client(1, None);
    let mut pfn_0_client = env.get_pfn_client(PUBLIC, 0, None);

    // some helpers for creation/minting
    let sender_account = testnet_dd_account_address();
    let creation_account = treasury_compliance_account_address();

    // Case 1:
    // submit client requests directly to VFN of dead V
    env.validator_swarm.kill_node(0);
    for _ in 0..2 {
        vfn_0_client.create_next_account(false).unwrap();
    }

    mint_coins(&mut vfn_0_client, 0, 100, XUS, true);
    mint_coins(&mut vfn_0_client, 1, 50, XUS, true);
    for _ in 0..8 {
        transfer_coins(&mut vfn_0_client, 0, 1, 1, XUS, false);
    }
    transfer_coins(&mut vfn_0_client, 0, 1, 1, XUS, true);

    // wait for VFN 1 to catch up with creation and sender account
    wait_for_transaction(&mut vfn_1_client, creation_account, 0);
    wait_for_transaction(&mut vfn_1_client, sender_account, 1);
    get_and_reset_sequence_number(&mut vfn_1_client, sender_account);
    get_and_reset_sequence_number(&mut vfn_1_client, creation_account);
    for _ in 0..4 {
        vfn_1_client.create_next_account(false).unwrap();
    }
    mint_coins(&mut vfn_1_client, 2, 100, XUS, true);
    mint_coins(&mut vfn_1_client, 3, 50, XUS, true);

    for _ in 0..6 {
        pfn_0_client.create_next_account(false).unwrap();
    }
    // wait for PFN to catch up with creation and sender account
    wait_for_transaction(&mut pfn_0_client, creation_account, 2);
    wait_for_transaction(&mut pfn_0_client, sender_account, 3);
    get_and_reset_sequence_number(&mut pfn_0_client, sender_account);
    get_and_reset_sequence_number(&mut pfn_0_client, creation_account);
    mint_coins(&mut pfn_0_client, 4, 100, XUS, true);
    mint_coins(&mut pfn_0_client, 5, 50, XUS, true);

    // bring down another V
    // Transition to unfortunate case where 2(>f) validators are down
    // and submit some transactions
    env.validator_swarm.kill_node(1);
    // submit some non-blocking txns during this scenario when >f validators are down
    for _ in 0..10 {
        transfer_coins(&mut vfn_1_client, 2, 3, 1, XUS, false);
    }

    // submit txn for vfn_0 too
    for _ in 0..5 {
        transfer_coins(&mut vfn_0_client, 0, 1, 1, XUS, false);
    }

    // we don't know which exact VFNs each pfn client's PFN is connected to,
    // but by pigeonhole principle, we know the PFN is connected to max 2 live VFNs
    for _ in 0..7 {
        transfer_coins(&mut pfn_0_client, 4, 5, 1, XUS, false);
    }

    // bring back one of the validators so consensus can resume
    assert!(env.validator_swarm.start_node(0).is_ok());
    // check all txns submitted so far (even those submitted during overlapping validator downtime) are committed

    let vfn_0_acct_0 = vfn_0_client.get_account(0).unwrap().address;
    wait_for_transaction(&mut vfn_0_client, vfn_0_acct_0, 13);
    let vfn_1_acct_0 = vfn_1_client.get_account(2).unwrap().address;
    wait_for_transaction(&mut vfn_1_client, vfn_1_acct_0, 9);
    let pfn_acct_0 = pfn_0_client.get_account(4).unwrap().address;
    wait_for_transaction(&mut pfn_0_client, pfn_acct_0, 6);

    // submit txns to vfn of dead V
    for _ in 0..5 {
        transfer_coins(&mut vfn_1_client, 2, 3, 1, XUS, false);
    }
    transfer_coins(&mut vfn_1_client, 2, 3, 1, XUS, true);

    // bring back all Vs back up
    assert!(env.validator_swarm.start_node(1).is_ok());

    // just for kicks: check regular minting still works with revived validators
    for _ in 0..5 {
        transfer_coins(&mut pfn_0_client, 4, 5, 1, XUS, false);
    }
    transfer_coins(&mut pfn_0_client, 4, 5, 1, XUS, true);
}

#[test]
fn test_private_full_node() {
    const PRIVATE: &str = "private";
    const USER: &str = "user";

    let mut env = SmokeTestEnvironment::new(7);
    env.setup_vfn_swarm();

    // Here we want to add two swarms, a private full node, followed by a user full node connected to it
    let mut private_config = NodeConfig::default_for_public_full_node();
    let private_network = private_config.full_node_networks.first_mut().unwrap();
    // Disallow public connections
    private_network.max_inbound_connections = 0;
    // Also, we only want it to purposely connect to 1 VFN
    private_network.max_outbound_connections = 1;

    let mut user_config = NodeConfig::default_for_public_full_node();
    let user_network = user_config.full_node_networks.first_mut().unwrap();
    // Disallow fallbacks to VFNs
    user_network.max_outbound_connections = 1;
    user_network.discovery_method = DiscoveryMethod::None;

    // The secret sauce, add the user as a downstream to the seeds
    add_node_to_seeds(
        &mut private_config,
        &user_config,
        NetworkId::Public,
        PeerRole::Downstream,
    );

    // Startup the Validator and VFN swarms so we can connect them
    env.validator_swarm.launch();
    let vfn_swarm = env.vfn_swarm();
    vfn_swarm.lock().launch();

    // Now we need to connect the VFNs to the private swarm
    add_node_to_seeds(
        &mut private_config,
        vfn_swarm.lock().get_node(0).unwrap().config(),
        NetworkId::Public,
        PeerRole::PreferredUpstream,
    );
    env.add_public_fn_swarm(PRIVATE, 1, Some(private_config), &vfn_swarm.lock().config);
    let private_swarm = env.public_swarm(PRIVATE);
    private_swarm.lock().launch();

    // And connect the user to the private swarm
    add_node_to_seeds(
        &mut user_config,
        private_swarm.lock().get_node(0).unwrap().config(),
        NetworkId::Public,
        PeerRole::PreferredUpstream,
    );
    env.add_public_fn_swarm(USER, 1, Some(user_config), &private_swarm.lock().config);
    let user_swarm = env.public_swarm(USER);

    user_swarm.lock().launch();

    // Ensure that User node is connected to private node and only the private node
    {
        let mut user_swarm = user_swarm.lock();
        let user_node = user_swarm.mut_node(0).unwrap();
        assert_eq!(
            1,
            user_node
                .get_connected_peers(NetworkId::Public, None)
                .unwrap_or(0),
            "User node is connected to more than one peer"
        );
    }

    // read state from full node client
    let mut validator_client = env.get_validator_client(0, None);
    let mut user_client = env.get_pfn_client(USER, 0, None);

    // mint from user node and check both validator and user node have correct balance
    let _account = validator_client.create_next_account(false).unwrap().address;
    user_client.create_next_account(false).unwrap();

    let sender_account = testnet_dd_account_address();
    let creation_account = treasury_compliance_account_address();

    get_and_reset_sequence_number(&mut user_client, sender_account);
    get_and_reset_sequence_number(&mut user_client, creation_account);
    mint_coins(&mut user_client, 0, 10, XUS, true);

    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut user_client, 0)
    ));

    let sequence = get_and_reset_sequence_number(&mut user_client, sender_account);
    wait_for_transaction(&mut validator_client, sender_account, sequence - 1);
    assert!(compare_balances(
        vec![(10.0, XUS.to_string())],
        get_balances(&mut validator_client, 0),
    ));
}

fn add_node_to_seeds(
    dest_config: &mut NodeConfig,
    seed_config: &NodeConfig,
    network_id: NetworkId,
    peer_role: PeerRole,
) {
    let dest_network_config = dest_config
        .full_node_networks
        .iter_mut()
        .find(|network| network.network_id == network_id)
        .unwrap();
    let seed_network_config = seed_config
        .full_node_networks
        .iter()
        .find(|network| network.network_id == network_id)
        .unwrap();

    let seed_peer_id = seed_network_config.peer_id();
    let seed_key = seed_network_config.identity_key().public_key();

    let seed_peer = if peer_role != PeerRole::Downstream {
        // For upstreams, we know the address, but so don't duplicate the keys in the config (lazy way)
        // TODO: This is ridiculous, we need a better way to manipulate these `NetworkAddress`s
        let address = seed_network_config.listen_address.clone();
        let port_protocol = address
            .as_slice()
            .iter()
            .find(|protocol| matches!(protocol, Protocol::Tcp(_)))
            .unwrap();
        let address = NetworkAddress::from(Protocol::Ip4(Ipv4Addr::new(127, 0, 0, 1)))
            .push(port_protocol.clone())
            .push(Protocol::NoiseIK(seed_key))
            .push(Protocol::Handshake(HANDSHAKE_VERSION));

        Peer::new(vec![address], HashSet::new(), peer_role)
    } else {
        // For downstreams, we don't know the address, but we know the keys
        let mut seed_keys = HashSet::new();
        seed_keys.insert(seed_key);
        Peer::new(vec![], seed_keys, peer_role)
    };

    dest_network_config.seeds.insert(seed_peer_id, seed_peer);
}
