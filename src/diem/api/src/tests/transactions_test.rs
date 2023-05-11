// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::tests::{assert_json, find_value, new_test_context, pretty, TestContext};

use diem_api_types::HexEncodedBytes;
use diem_crypto::{
    hash::CryptoHash,
    multi_ed25519::{MultiEd25519PrivateKey, MultiEd25519PublicKey},
    SigningKey, Uniform,
};
use diem_sdk::{client::SignedTransaction, transaction_builder::Currency, types::LocalAccount};
use diem_types::{
    access_path::{AccessPath, Path},
    account_address::AccountAddress,
    account_config::{from_currency_code_string, xus_tag, XUS_NAME},
    transaction::{
        authenticator::{AuthenticationKey, TransactionAuthenticator},
        ChangeSet, Script, ScriptFunction, Transaction, TransactionInfoTrait,
    },
    write_set::{WriteOp, WriteSetMut},
};

use move_core_types::{
    identifier::Identifier,
    language_storage::{ModuleId, StructTag, TypeTag, CORE_CODE_ADDRESS},
};
use rand::{distributions::Alphanumeric, thread_rng, Rng};
use serde_json::json;

#[tokio::test]
async fn test_get_transactions_output_genesis_transaction() {
    let context = new_test_context();
    let ledger_info = context.get_latest_ledger_info();
    let txns = context
        .context
        .get_transactions(0, 1, ledger_info.version())
        .unwrap();

    let resp = context.get("/transactions").await;
    assert_eq!(1, resp.as_array().unwrap().len());
    let txn = &resp[0];
    assert_eq!(txn["type"], "genesis_transaction");
    assert_eq!(txn["version"], "0");

    let info = txns[0].info.clone();
    assert_eq!(txn["hash"], info.transaction_hash().to_hex_literal());
    assert_eq!(
        txn["state_root_hash"],
        info.state_root_hash().to_hex_literal()
    );
    assert_eq!(
        txn["event_root_hash"],
        info.event_root_hash().to_hex_literal()
    );
    let chain_id = find_value(&txn["payload"]["write_set"]["changes"], |val| {
        val["type"] == "write_module" && val["data"]["abi"]["name"] == "ChainId"
    });
    let bytecode = chain_id["data"]["bytecode"].clone();
    assert!(bytecode.as_str().unwrap().starts_with("0x"));
    assert_json(
        chain_id,
        json!({
            "type": "write_module",
            "address": "0x1",
            "data": {
                "bytecode": bytecode.as_str().unwrap(),
                "abi": {
                    "address": "0x1",
                    "name": "ChainId",
                    "friends": [],
                    "exposed_functions": [
                        {
                            "name": "get",
                            "visibility": "public",
                            "generic_type_params": [],
                            "params": [],
                            "return": ["u8"]
                        },
                        {
                            "name": "initialize",
                            "visibility": "public",
                            "generic_type_params": [],
                            "params": [
                                "&signer",
                                "u8"
                            ],
                            "return": []
                        }
                    ],
                    "structs": [
                        {
                            "name": "ChainId",
                            "is_native": false,
                            "abilities": [
                                "key"
                            ],
                            "generic_type_params": [],
                            "fields": [
                                {
                                    "name": "id",
                                    "type": "u8"
                                }
                            ]
                        }
                    ]
                }
            }
        }),
    );

    let chain_id = find_value(&txn["payload"]["write_set"]["changes"], |val| {
        val["type"] == "write_resource"
            && val["address"] == "0xdd"
            && val["data"]["type"] == "0x1::Roles::RoleId"
    });
    assert_json(
        chain_id,
        json!({
            "type": "write_resource",
            "address": "0xdd",
            "data": {
                "type": "0x1::Roles::RoleId",
                "data": {
                    "role_id": "2"
                }
            }
        }),
    );

    let first_event = txn["events"][0].clone();
    // transaction events are same with events from payload
    assert_json(
        first_event.clone(),
        txn["payload"]["write_set"]["events"][0].clone(),
    );
    assert_json(
        first_event,
        json!({
            "key": "0x00000000000000000000000000000000000000000a550c18",
            "sequence_number": "0",
            "type": "0x1::DiemAccount::CreateAccountEvent",
            "data": {
                "created": "0xa550c18",
                "role_id": "0"
            }
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_with_start_version_is_too_large() {
    let context = new_test_context();
    let ledger_version = context.get_latest_ledger_info().version();
    let resp = context
        .expect_status_code(404)
        .get("/transactions?start=1000000&limit=10")
        .await;
    assert_json(
        resp,
        json!({
          "code": 404,
          "message": "transaction not found by version(1000000)",
          "diem_ledger_version": ledger_version.to_string()
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_with_invalid_start_version_param() {
    let context = new_test_context();
    let resp = context
        .expect_status_code(400)
        .get("/transactions?start=hello")
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid parameter start: hello"
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_with_invalid_limit_param() {
    let context = new_test_context();
    let resp = context
        .expect_status_code(400)
        .get("/transactions?limit=hello")
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid parameter limit: hello"
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_with_zero_limit() {
    let context = new_test_context();
    let resp = context
        .expect_status_code(400)
        .get("/transactions?limit=0")
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid parameter limit: 0"
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_param_limit_exceeds_limit() {
    let context = new_test_context();
    let resp = context
        .expect_status_code(400)
        .get("/transactions?limit=2000")
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid parameter limit: 2000, exceed limit 1000"
        }),
    );
}

#[tokio::test]
async fn test_get_transactions_output_user_transaction_with_script_function_payload() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=1").await;
    assert_eq!(2, txns.as_array().unwrap().len());

    let expected_txns = context.get_transactions(1, 2);
    assert_eq!(2, expected_txns.len());

    let metadata = expected_txns[0].info.clone();

    let metadata_txn = match &expected_txns[0].transaction {
        Transaction::BlockMetadata(txn) => txn.clone(),
        _ => panic!("unexpected transaction: {:?}", expected_txns[0].transaction),
    };
    assert_json(
        txns[0].clone(),
        json!(
        {
            "type": "block_metadata_transaction",
            "version": "1",
            "hash": metadata.transaction_hash().to_hex_literal(),
            "state_root_hash": metadata.state_root_hash().to_hex_literal(),
            "event_root_hash": metadata.event_root_hash().to_hex_literal(),
            "gas_used": metadata.gas_used().to_string(),
            "success": true,
            "vm_status": "Executed successfully",
            "id": metadata_txn.id().to_hex_literal(),
            "round": "1",
            "previous_block_votes": [],
            "proposer": context.validator_owner.to_hex_literal(),
            "timestamp": metadata_txn.timestamp_usec().to_string(),
        }),
    );

    let user_txn_info = expected_txns[1].info.clone();
    let (public_key, sig) = match txn.authenticator() {
        TransactionAuthenticator::Ed25519 {
            public_key,
            signature,
        } => (public_key, signature),
        _ => panic!(
            "expecting TransactionAuthenticator::Ed25519, but got: {:?}",
            txn.authenticator()
        ),
    };
    assert_json(
        txns[1].clone(),
        json!({
            "type": "user_transaction",
            "version": "2",
            "hash": user_txn_info.transaction_hash().to_hex_literal(),
            "state_root_hash": user_txn_info.state_root_hash().to_hex_literal(),
            "event_root_hash": user_txn_info.event_root_hash().to_hex_literal(),
            "gas_used": user_txn_info.gas_used().to_string(),
            "success": true,
            "vm_status": "Executed successfully",
            "sender": "0xb1e55ed",
            "sequence_number": "0",
            "max_gas_amount": "1000000",
            "gas_unit_price": "0",
            "gas_currency_code": "XUS",
            "expiration_timestamp_secs": txn.expiration_timestamp_secs().to_string(),
            "events": [
                {
                    "key": "0x00000000000000000000000000000000000000000a550c18",
                    "sequence_number": "5",
                    "type": "0x1::DiemAccount::CreateAccountEvent",
                    "data": {
                        "created": account.address().to_hex_literal(),
                        "role_id": "5"
                    }
                }
            ],
            "payload": {
                "type": "script_function_payload",
                "function": "0x1::AccountCreationScripts::create_parent_vasp_account",
                "type_arguments": [
                    "0x1::XUS::XUS"
                ],
                "arguments": [
                    "0",
                    account.address().to_hex_literal(),
                    format!("0x{}", hex::encode(account.authentication_key().prefix())),
                    format!("0x{}", hex::encode("vasp".as_bytes())),
                    true
                ]
            },
            "signature": {
                "type": "ed25519_signature",
                "public_key": format!("0x{}", hex::encode(public_key.unvalidated().to_bytes())),
                "signature": format!("0x{}", hex::encode(sig.to_bytes())),
            },
        }),
    )
}

#[tokio::test]
async fn test_get_transactions_output_user_transaction_with_script_payload() {
    let context = new_test_context();
    let new_key = "717d1d400311ff8797c2441ea9c2d2da1120ce38f66afb079c2bad0919d93a09"
        .parse()
        .unwrap();

    let mut tc_account = context.tc_account();
    let txn = tc_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .rotate_authentication_key_by_script(new_key),
    );
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=2").await;
    assert_eq!(1, txns.as_array().unwrap().len());

    let expected_txns = context.get_transactions(2, 1);
    assert_eq!(1, expected_txns.len());

    assert_json(
        txns[0]["payload"].clone(),
        json!({
            "type": "script_payload",
            "code": {
                "bytecode": "0xa11ceb0b010000000601000202020403060f05151207277c08a3011000000001010000020001000003010200000403020001060c01080000020608000a0202060c0a020b4469656d4163636f756e74154b6579526f746174696f6e4361706162696c6974791f657874726163745f6b65795f726f746174696f6e5f6361706162696c6974791f726573746f72655f6b65795f726f746174696f6e5f6361706162696c69747919726f746174655f61757468656e7469636174696f6e5f6b657900000000000000000000000000000001000401090b0011000c020e020b0111020b02110102",
                "abi": {
                    "name": "main",
                    "visibility": "script",
                    "generic_type_params": [],
                    "params": [
                        "&signer",
                        "vector<u8>"
                    ],
                    "return": []
                }
            },
            "type_arguments": [],
            "arguments": [
                "0x717d1d400311ff8797c2441ea9c2d2da1120ce38f66afb079c2bad0919d93a09"
            ]
        }),
    )
}

#[tokio::test]
async fn test_get_transactions_output_user_transaction_with_module_payload() {
    let context = new_test_context();
    let code = "a11ceb0b0300000006010002030205050703070a0c0816100c260900000001000100000102084d794d6f64756c650269640000000000000000000000000b1e55ed00010000000231010200";
    let mut tc_account = context.tc_account();
    let txn = tc_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .module(hex::decode(code).unwrap()),
    );
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=2").await;
    assert_eq!(1, txns.as_array().unwrap().len());

    let expected_txns = context.get_transactions(2, 1);
    assert_eq!(1, expected_txns.len());

    assert_json(
        txns[0]["payload"].clone(),
        json!({
            "type": "module_bundle_payload",
            "modules": [
                {
                    "bytecode": format!("0x{}", code),
                    "abi": {
                        "address": "0xb1e55ed",
                        "name": "MyModule",
                        "friends": [],
                        "exposed_functions": [
                            {
                                "name": "id",
                                "visibility": "public",
                                "generic_type_params": [],
                                "params": [],
                                "return": ["u8"]
                            }
                        ],
                        "structs": []
                    }
                },
            ]
        }),
    )
}

#[tokio::test]
async fn test_get_transactions_output_user_transaction_with_write_set_payload() {
    let context = new_test_context();
    let mut root_account = context.root_account();
    let code_address = AccountAddress::from_hex_literal("0x1").unwrap();
    let txn = root_account.sign_with_transaction_builder(
        context.transaction_factory().change_set(ChangeSet::new(
            WriteSetMut::new(vec![
                (
                    AccessPath::new(
                        code_address,
                        bcs::to_bytes(&Path::Code(ModuleId::new(
                            code_address,
                            Identifier::new("AccountAdministrationScripts").unwrap(),
                        )))
                        .unwrap(),
                    ),
                    WriteOp::Deletion,
                ),
                (
                    AccessPath::new(
                        context.tc_account().address(),
                        bcs::to_bytes(&Path::Resource(StructTag {
                            address: code_address,
                            module: Identifier::new("AccountFreezing").unwrap(),
                            name: Identifier::new("FreezingBit").unwrap(),
                            type_params: vec![],
                        }))
                        .unwrap(),
                    ),
                    WriteOp::Deletion,
                ),
            ])
            .freeze()
            .unwrap(),
            vec![],
        )),
    );
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=2").await;
    assert_eq!(1, txns.as_array().unwrap().len());

    assert_json(
        txns[0]["payload"].clone(),
        json!({
            "type": "write_set_payload",
            "write_set": {
                "type": "direct_write_set",
                "changes": [
                    {
                        "type": "delete_module",
                        "address": "0x1",
                        "module": "0x1::AccountAdministrationScripts"
                    },
                    {
                        "type": "delete_resource",
                        "address": "0xb1e55ed",
                        "resource": "0x1::AccountFreezing::FreezingBit"
                    }
                ],
                "events": []
            }
        }),
    )
}

#[tokio::test]
async fn test_post_bcs_format_transaction() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    let body = bcs::to_bytes(&txn).unwrap();
    let resp = context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", body)
        .await;
    let expiration_timestamp = txn.expiration_timestamp_secs();
    let (public_key, sig) = match txn.authenticator() {
        TransactionAuthenticator::Ed25519 {
            public_key,
            signature,
        } => (public_key, signature),
        _ => panic!(
            "expecting TransactionAuthenticator::Ed25519, but got: {:?}",
            txn.authenticator()
        ),
    };
    let hash = Transaction::UserTransaction(txn).hash();
    assert_json(
        resp.clone(),
        json!({
            "type": "pending_transaction",
            "hash": hash.to_hex_literal(),
            "sender": "0xb1e55ed",
            "sequence_number": "0",
            "max_gas_amount": "1000000",
            "gas_unit_price": "0",
            "gas_currency_code": "XUS",
            "expiration_timestamp_secs": expiration_timestamp.to_string(),
            "payload": {
                "type": "script_function_payload",
                "function": "0x1::AccountCreationScripts::create_parent_vasp_account",
                "type_arguments": [
                    "0x1::XUS::XUS"
                ],
                "arguments": [
                    "0",
                    account.address().to_hex_literal(),
                    format!("0x{}", hex::encode(account.authentication_key().prefix())),
                    format!("0x{}", hex::encode("vasp".as_bytes())),
                    true
                ]
            },
            "signature": {
                "type": "ed25519_signature",
                "public_key": format!("0x{}", hex::encode(public_key.unvalidated().to_bytes())),
                "signature": format!("0x{}", hex::encode(sig.to_bytes())),
            },
        }),
    );

    // ensure ed25519 sig txn can be submitted into mempool by JSON format
    context
        .expect_status_code(202)
        .post("/transactions", resp)
        .await;
}

#[tokio::test]
async fn test_post_invalid_bcs_format_transaction() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(400)
        .post_bcs_txn("/transactions", bcs::to_bytes("invalid data").unwrap())
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid request body: deserialize error: unexpected end of input"
        }),
    );
}

#[tokio::test]
async fn test_post_invalid_signature_transaction() {
    let mut context = new_test_context();
    let txn = context.create_invalid_signature_transaction();
    let body = bcs::to_bytes(&txn).unwrap();
    let resp = context
        .expect_status_code(400)
        .post_bcs_txn("/transactions", &body)
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "invalid transaction: INVALID_SIGNATURE"
        }),
    );
}

#[tokio::test]
async fn test_post_transaction_rejected_by_mempool() {
    let mut context = new_test_context();
    let account1 = context.gen_account();
    let account2 = context.gen_account();
    let txn1 = context.create_parent_vasp(&account1);
    let txn2 = context.create_parent_vasp(&account2);

    context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", &bcs::to_bytes(&txn1).unwrap())
        .await;

    let resp = context
        .expect_status_code(400)
        .post_bcs_txn("/transactions", &bcs::to_bytes(&txn2).unwrap())
        .await;
    assert_json(
        resp,
        json!({
          "code": 400,
          "message": "transaction is rejected: InvalidUpdate - Transaction already in mempool"
        }),
    );
}

#[tokio::test]
async fn test_multi_agent_signed_transaction() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let factory = context.transaction_factory();
    let mut tc_account = context.tc_account();
    let secondary = context.root_account();
    let txn = tc_account.sign_multi_agent_with_transaction_builder(
        vec![&secondary],
        factory.create_parent_vasp_account(
            Currency::XUS,
            0,
            account.authentication_key(),
            "vasp",
            true,
        ),
    );

    let body = bcs::to_bytes(&txn).unwrap();
    let resp = context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", body)
        .await;

    let (sender, secondary_signers) = match txn.authenticator() {
        TransactionAuthenticator::MultiAgent {
            sender,
            secondary_signer_addresses: _,
            secondary_signers,
        } => (sender, secondary_signers),
        _ => panic!(
            "expecting TransactionAuthenticator::MultiAgent, but got: {:?}",
            txn.authenticator()
        ),
    };
    assert_json(
        resp["signature"].clone(),
        json!({
            "type": "multi_agent_signature",
            "sender": {
                "type": "ed25519_signature",
                "public_key": format!("0x{}", hex::encode(sender.public_key_bytes())),
                "signature": format!("0x{}", hex::encode(sender.signature_bytes())),
            },
            "secondary_signer_addresses": [
                secondary.address().to_hex_literal(),
            ],
            "secondary_signers": [
                {
                    "type": "ed25519_signature",
                    "public_key": format!("0x{}",hex::encode(secondary_signers[0].public_key_bytes())),
                    "signature": format!("0x{}", hex::encode(secondary_signers[0].signature_bytes())),
                }
            ]
        }),
    );

    // ensure multi agent txns can be submitted into mempool by JSON format
    context
        .expect_status_code(202)
        .post("/transactions", resp)
        .await;
}

#[tokio::test]
async fn test_multi_ed25519_signed_transaction() {
    let context = new_test_context();

    let private_key = MultiEd25519PrivateKey::generate_for_testing();
    let public_key = MultiEd25519PublicKey::from(&private_key);
    let auth_key = AuthenticationKey::multi_ed25519(&public_key);

    let factory = context.transaction_factory();
    let mut tc_account = context.tc_account();
    let create_account_txn = tc_account.sign_with_transaction_builder(
        factory.create_parent_vasp_account(Currency::XUS, 0, auth_key, "vasp", true),
    );
    context.commit_block(&vec![create_account_txn]).await;

    let raw_txn = factory
        .create_recovery_address()
        .sender(auth_key.derived_address())
        .sequence_number(0)
        .expiration_timestamp_secs(u64::MAX) // set timestamp to max to ensure static raw transaction
        .build();

    let signature = private_key.sign(&raw_txn);
    let txn = SignedTransaction::new_multisig(raw_txn, public_key, signature.clone());

    let body = bcs::to_bytes(&txn).unwrap();
    let resp = context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", body)
        .await;

    assert_json(
        resp["signature"].clone(),
        json!({
          "type": "multi_ed25519_signature",
          "public_keys": [
            "0x9e4208caddd825f71957c9b12dbfbd13a23fb0ea23eb398fd7e1f418b51f8fbc",
            "0x4708a77bb9285ce3745ffdd48c51980326b625488209803228ff623f3768c64e",
            "0x852b13cd7a89b0c223d74504705e84c745d32261244ed233ef0285637a1dece0",
            "0x77e7fe2a510e4f14e15071fc420469ee287b64f2c8f8c0221b946a3fd9cbfef3",
            "0xd0c66cfef88b999f027347726bd54eda4675ae312af9146bfdc9e9fa702cc90a",
            "0xd316059933e0dd6415f00ce350962c8e94b46373b7fb5fb49687f3d6b9e3cb30",
            "0xf20e973e6dfeda74ca8e15f1a7aed9c87d67bd12e071fd3de4240368422712c9",
            "0xead82d6e9e3f3baeaa557bd7a431a1c6fe9f35a82c10fed123f362615ee7c2cd",
            "0x5c048c8c456ff9dd2810343bbd630fb45bf064317efae22c65a1535cf392c5d5",
            "0x861546d0818178f2b5f37af0fa712fe8ce3cceeda894b553ee274f3fbcb4b32f",
            "0xfe047a766a47719591348a4601afb3f38b0c77fa3f820e0298c064e7cde6763f"
          ],
          "signatures": [
            "0xab0ffa0926dd765979c422572b4429d11161a2df6975e223ad4d75c87a117e6c790558e8286caf95550ab97515d2cfa8654365f54524688df91b3b4e91b69d0e",
            "0x300774b6dd50658d4b693ad5cc1842944465a92b31f1652b445d36b911d4ca625260c451ab7d998534b61253f3bfcdd6bcb03adf4c048b03bd18678d56cd5a03",
            "0x4bac0f0d9dde41196efae43849f8e4427ee142e04e57e7291ecdfb225528b0fe31eff8e17461a220430daea94a14f750a37b5e0360aa1c72cb956c402743c202"
          ],
          "threshold": 3,
          "bitmap": "0xe0000000"
        }),
    );

    // ensure multi sig txns can be submitted into mempool by JSON format
    context
        .expect_status_code(202)
        .post("/transactions", resp)
        .await;
}

#[tokio::test]
async fn test_get_transaction_by_hash() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=2").await;
    assert_eq!(1, txns.as_array().unwrap().len());

    let resp = context
        .get(&format!(
            "/transactions/{}",
            txns[0]["hash"].as_str().unwrap()
        ))
        .await;
    assert_json(resp, txns[0].clone())
}

#[tokio::test]
async fn test_get_transaction_by_hash_not_found() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(404)
        .get("/transactions/0xdadfeddcca7cb6396c735e9094c76c6e4e9cb3e3ef814730693aed59bd87b31d")
        .await;
    assert_json(
        resp,
        json!({
            "code": 404,
            "message": "transaction not found by hash(0xdadfeddcca7cb6396c735e9094c76c6e4e9cb3e3ef814730693aed59bd87b31d)",
            "diem_ledger_version": "0"
        }),
    )
}

#[tokio::test]
async fn test_get_transaction_by_invalid_hash() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(400)
        .get(&format!("/transactions/{}", "0x1",))
        .await;
    assert_json(
        resp,
        json!({
            "code": 400,
            "message": "invalid parameter transaction hash or version: 0x1"
        }),
    )
}

#[tokio::test]
async fn test_get_transaction_by_version_not_found() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(404)
        .get("/transactions/10000")
        .await;
    assert_json(
        resp,
        json!({
            "code": 404,
            "message": "transaction not found by version(10000)",
            "diem_ledger_version": "0"
        }),
    )
}

#[tokio::test]
async fn test_get_transaction_by_version() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn.clone()]).await;

    let txns = context.get("/transactions?start=2").await;
    assert_eq!(1, txns.as_array().unwrap().len());

    let resp = context.get("/transactions/2").await;
    assert_json(resp, txns[0].clone())
}

#[tokio::test]
async fn test_get_pending_transaction_by_hash() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    let body = bcs::to_bytes(&txn).unwrap();
    let pending_txn = context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", body)
        .await;

    let txn_hash = pending_txn["hash"].as_str().unwrap();

    let txn = context.get(&format!("/transactions/{}", txn_hash)).await;
    assert_json(txn, pending_txn);

    let not_found = context
        .expect_status_code(404)
        .get("/transactions/0xdadfeddcca7cb6396c735e9094c76c6e4e9cb3e3ef814730693aed59bd87b31d")
        .await;
    assert_json(
        not_found,
        json!({
            "code": 404,
            "message": "transaction not found by hash(0xdadfeddcca7cb6396c735e9094c76c6e4e9cb3e3ef814730693aed59bd87b31d)",
            "diem_ledger_version": "0"
        }),
    )
}

#[tokio::test]
async fn test_signing_message_with_script_function_payload() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);

    let payload = json!({
        "type": "script_function_payload",
        "function": "0x1::AccountCreationScripts::create_parent_vasp_account",
        "type_arguments": [
            "0x1::XUS::XUS"
        ],
        "arguments": [
            "0",     // sliding_nonce
            account.address().to_hex_literal(), // new_account_address
            format!("0x{}", hex::encode(account.authentication_key().prefix())), // auth_key_prefix
            format!("0x{}", hex::encode("vasp".as_bytes())), // human_name
            true, // add_all_currencies
        ]
    });
    test_signing_message_with_payload(context, txn, payload).await;
}

#[tokio::test]
async fn test_signing_message_with_module_payload() {
    let context = new_test_context();
    let code = "a11ceb0b0300000006010002030205050703070a0c0816100c260900000001000100000102084d794d6f64756c650269640000000000000000000000000b1e55ed00010000000231010200";
    let mut tc_account = context.tc_account();
    let txn = tc_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .module(hex::decode(code).unwrap()),
    );
    let payload = json!({
            "type": "module_bundle_payload",
            "modules" : [
                {"bytecode": format!("0x{}", code)},
            ],
    });

    test_signing_message_with_payload(context, txn, payload).await;
}

#[tokio::test]
async fn test_signing_message_with_script_payload() {
    let context = new_test_context();
    let new_key = "717d1d400311ff8797c2441ea9c2d2da1120ce38f66afb079c2bad0919d93a09"
        .parse()
        .unwrap();

    let mut tc_account = context.tc_account();
    let txn = tc_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .rotate_authentication_key_by_script(new_key),
    );

    let code = "a11ceb0b010000000601000202020403060f05151207277c08a3011000000001010000020001000003010200000403020001060c01080000020608000a0202060c0a020b4469656d4163636f756e74154b6579526f746174696f6e4361706162696c6974791f657874726163745f6b65795f726f746174696f6e5f6361706162696c6974791f726573746f72655f6b65795f726f746174696f6e5f6361706162696c69747919726f746174655f61757468656e7469636174696f6e5f6b657900000000000000000000000000000001000401090b0011000c020e020b0111020b02110102";
    let payload = json!({
            "type": "script_payload",
            "code": {
                "bytecode": format!("0x{}", code)
            },
            "type_arguments": [],
            "arguments": [
                "0x717d1d400311ff8797c2441ea9c2d2da1120ce38f66afb079c2bad0919d93a09"
            ]
    });

    test_signing_message_with_payload(context, txn, payload).await;
}

#[tokio::test]
async fn test_signing_message_with_write_set_payload() {
    // This test is created for testing error message for now.
    // Update test when write_set_payload is supported
    let context = new_test_context();
    let mut root_account = context.root_account();
    let code_address = AccountAddress::from_hex_literal("0x1").unwrap();
    let txn = root_account.sign_with_transaction_builder(
        context.transaction_factory().change_set(ChangeSet::new(
            WriteSetMut::new(vec![
                (
                    AccessPath::new(
                        code_address,
                        bcs::to_bytes(&Path::Code(ModuleId::new(
                            code_address,
                            Identifier::new("AccountAdministrationScripts").unwrap(),
                        )))
                        .unwrap(),
                    ),
                    WriteOp::Deletion,
                ),
                (
                    AccessPath::new(
                        context.tc_account().address(),
                        bcs::to_bytes(&Path::Resource(StructTag {
                            address: code_address,
                            module: Identifier::new("AccountFreezing").unwrap(),
                            name: Identifier::new("FreezingBit").unwrap(),
                            type_params: vec![],
                        }))
                        .unwrap(),
                    ),
                    WriteOp::Deletion,
                ),
            ])
            .freeze()
            .unwrap(),
            vec![],
        )),
    );
    let payload = json!({
        "type": "write_set_payload",
        "write_set": {
            "type": "direct_write_set",
            "changes": [
                {
                    "type": "delete_module",
                    "address": "0x1",
                    "module": "0x1::AccountAdministrationScripts"
                },
                {
                    "type": "delete_resource",
                    "address": "0xb1e55ed",
                    "resource": "0x1::AccountFreezing::FreezingBit"
                }
            ],
            "events": []
        }
    });

    let sender = context.tc_account();
    let body = json!({
        "sender": sender.address().to_hex_literal(),
        "sequence_number": sender.sequence_number().to_string(),
        "gas_unit_price": txn.gas_unit_price().to_string(),
        "max_gas_amount": txn.max_gas_amount().to_string(),
        "gas_currency_code": txn.gas_currency_code(),
        "expiration_timestamp_secs": txn.expiration_timestamp_secs().to_string(),
        "payload": payload,
    });

    context
        .expect_status_code(400)
        .post("/transactions/signing_message", body)
        .await;
}

async fn test_signing_message_with_payload(
    context: TestContext,
    txn: SignedTransaction,
    payload: serde_json::Value,
) {
    let sender = context.tc_account();
    let mut body = json!({
        "sender": sender.address().to_hex_literal(),
        "sequence_number": sender.sequence_number().to_string(),
        "gas_unit_price": txn.gas_unit_price().to_string(),
        "max_gas_amount": txn.max_gas_amount().to_string(),
        "gas_currency_code": txn.gas_currency_code(),
        "expiration_timestamp_secs": txn.expiration_timestamp_secs().to_string(),
        "payload": payload,
    });

    let resp = context
        .post("/transactions/signing_message", body.clone())
        .await;

    let signing_msg = resp["message"].as_str().unwrap();
    assert_eq!(
        signing_msg,
        format!(
            "0x{}",
            hex::encode(&txn.clone().into_raw_transaction().signing_message())
        )
    );

    let hex_bytes: HexEncodedBytes = signing_msg.parse().unwrap();
    let sig = context
        .tc_account()
        .private_key()
        .sign_arbitrary_message(hex_bytes.inner());
    let expected_sig = match txn.authenticator() {
        TransactionAuthenticator::Ed25519 {
            public_key: _,
            signature,
        } => signature,
        _ => panic!("expect TransactionAuthenticator::Ed25519"),
    };
    assert_eq!(sig, expected_sig);

    // assert transaction can be submitted into mempool and execute.
    body["signature"] = json!({
        "type": "ed25519_signature",
        "public_key": format!("0x{}", hex::encode(sender.public_key().to_bytes())),
        "signature": format!("0x{}", hex::encode(sig.to_bytes())),
    });

    context
        .expect_status_code(202)
        .post("/transactions", body)
        .await;

    context.commit_mempool_txns(10).await;

    let ledger = context.get("/").await;
    assert_eq!(ledger["ledger_version"].as_str().unwrap(), "2"); // one metadata + one txn
}

#[tokio::test]
async fn test_get_account_transactions() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn]).await;

    let txns = context
        .get(format!("/accounts/{}/transactions", context.tc_account().address()).as_str())
        .await;
    assert_eq!(1, txns.as_array().unwrap().len());
    let expected_txns = context.get("/transactions?start=2&limit=1").await;
    assert_json(txns, expected_txns);
}

#[tokio::test]
async fn test_get_account_transactions_filter_transactions_by_start_sequence_number() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn]).await;

    let txns = context
        .get(
            format!(
                "/accounts/{}/transactions?start=1",
                context.tc_account().address()
            )
            .as_str(),
        )
        .await;
    assert_json(txns, json!([]));
}

#[tokio::test]
async fn test_get_account_transactions_filter_transactions_by_start_sequence_number_is_too_large() {
    let mut context = new_test_context();
    let account = context.gen_account();
    let txn = context.create_parent_vasp(&account);
    context.commit_block(&vec![txn]).await;

    let txns = context
        .get(
            format!(
                "/accounts/{}/transactions?start=1000",
                context.tc_account().address()
            )
            .as_str(),
        )
        .await;
    assert_json(txns, json!([]));
}

#[tokio::test]
async fn test_get_account_transactions_filter_transactions_by_limit() {
    let mut context = new_test_context();
    let mut tc_account = context.tc_account();
    let account1 = context.gen_account();
    let txn1 = context.create_parent_vasp_by_account(&mut tc_account, &account1);
    let account2 = context.gen_account();
    let txn2 = context.create_parent_vasp_by_account(&mut tc_account, &account2);
    context.commit_block(&vec![txn1, txn2]).await;

    let txns = context
        .get(
            format!(
                "/accounts/{}/transactions?start=0&limit=1",
                context.tc_account().address()
            )
            .as_str(),
        )
        .await;
    assert_eq!(txns.as_array().unwrap().len(), 1);

    let txns = context
        .get(
            format!(
                "/accounts/{}/transactions?start=0&limit=2",
                context.tc_account().address()
            )
            .as_str(),
        )
        .await;
    assert_eq!(txns.as_array().unwrap().len(), 2);
}

const MISC_ERROR: &str = "Move bytecode deserialization / verification failed, including script function not found or invalid arguments";

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_module_payload_bytecode() {
    let context = new_test_context();
    let invalid_bytecode = hex::decode("a11ceb0b030000").unwrap();
    let mut tc_account = context.tc_account();
    let txn = tc_account
        .sign_with_transaction_builder(context.transaction_factory().module(invalid_bytecode));
    test_transaction_vm_status(context, txn, false, MISC_ERROR).await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_payload_bytecode() {
    let context = new_test_context();
    let mut tc_account = context.tc_account();
    let invalid_bytecode = hex::decode("a11ceb0b030000").unwrap();
    let txn = tc_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .script(Script::new(invalid_bytecode, vec![], vec![])),
    );
    test_transaction_vm_status(context, txn, false, MISC_ERROR).await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_write_set_payload() {
    let context = new_test_context();

    let invalid_bytecode = hex::decode("a11ceb0b030000").unwrap();
    let mut root_account = context.root_account();
    let code_address = AccountAddress::from_hex_literal("0x1").unwrap();
    let txn = root_account.sign_with_transaction_builder(
        context.transaction_factory().change_set(ChangeSet::new(
            WriteSetMut::new(vec![(
                AccessPath::new(
                    code_address,
                    bcs::to_bytes(&Path::Code(ModuleId::new(
                        code_address,
                        Identifier::new("AccountAdministrationScripts").unwrap(),
                    )))
                    .unwrap(),
                ),
                WriteOp::Value(invalid_bytecode),
            )])
            .freeze()
            .unwrap(),
            vec![],
        )),
    );

    // should fail, but VM executed successfully, need investigate, but out of API scope
    test_transaction_vm_status(context, txn, true, "Executed successfully").await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_function_address() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1222",
        "PaymentScripts",
        "peer_to_peer_with_metadata",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&1u64).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_function_module_name() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScriptsInvalid",
        "peer_to_peer_with_metadata",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&1u64).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_function_name() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScripts",
        "peer_to_peer_with_metadata_invalid",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&1u64).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_function_type_arguments() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScripts",
        "peer_to_peer_with_metadata_invalid",
        vec![TypeTag::Struct(StructTag {
            address: CORE_CODE_ADDRESS,
            module: from_currency_code_string(XUS_NAME).unwrap(),
            name: Identifier::new("invalid").unwrap(),
            type_params: vec![],
        })],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&1u64).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_invalid_script_function_arguments() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScripts",
        "peer_to_peer_with_metadata",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&1u8).unwrap(), // invalid type
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_missing_script_function_arguments() {
    let context = new_test_context();
    let account = context.dd_account();
    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScripts",
        "peer_to_peer_with_metadata",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            // missing 3 arguments
        ],
        MISC_ERROR,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_script_function_validation() {
    let mut context = new_test_context();
    let account = context.gen_account();
    context
        .commit_block(&vec![context.create_parent_vasp(&account)])
        .await;

    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        account,
        "0x1",
        "PaymentScripts",
        "peer_to_peer_with_metadata",
        vec![xus_tag()],
        vec![
            bcs::to_bytes(&AccountAddress::from_hex_literal("0xdd").unwrap()).unwrap(),
            bcs::to_bytes(&123u64).unwrap(), // exceed limit, account balance is 0.
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
            bcs::to_bytes(&Vec::<u8>::new()).unwrap(),
        ],
        r#"Move abort by LIMIT_EXCEEDED - EINSUFFICIENT_BALANCE
 A limit on an amount, e.g. a currency, is exceeded. Example: withdrawal of money after account limits window
 is exhausted.
 The account does not hold a large enough balance in the specified currency"#,
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_script_function_execution_failure() {
    let context = new_test_context();

    // address 0xA550C18 {
    //     module Hello {
    //         fun world() {
    //             1/0;
    //         }
    //         public(script) fun hello() {
    //             world();
    //         }
    //     }
    // }
    let hello_script_fun = hex::decode("a11ceb0b030000000601000203020a050c01070d12081f100c2f24000000010000000002000000000548656c6c6f0568656c6c6f05776f726c640000000000000000000000000a550c180002000000021101020100000000050601000000000000000600000000000000001a010200").unwrap();
    let mut root_account = context.root_account();
    let module_txn = root_account
        .sign_with_transaction_builder(context.transaction_factory().module(hello_script_fun));

    context.commit_block(&vec![module_txn]).await;

    test_get_txn_execute_failed_by_invalid_script_function(
        context,
        root_account,
        "0xA550C18",
        "Hello",
        "hello",
        vec![],
        vec![],
        "Execution failed in 0000000000000000000000000A550C18::Hello::world at code offset 2",
    )
    .await
}

#[tokio::test]
async fn test_get_txn_execute_failed_by_script_execution_failure() {
    let context = new_test_context();

    // script {
    //     fun main() {
    //         1/0;
    //     }
    // }
    let script =
        hex::decode("a11ceb0b030000000105000100000000050601000000000000000600000000000000001a0102")
            .unwrap();
    let mut root_account = context.root_account();
    let txn = root_account.sign_with_transaction_builder(
        context
            .transaction_factory()
            .script(Script::new(script, vec![], vec![])),
    );

    test_transaction_vm_status(
        context,
        txn,
        false,
        "Execution failed in script at code offset 2",
    )
    .await
}

async fn test_get_txn_execute_failed_by_invalid_script_function(
    context: TestContext,
    mut account: LocalAccount,
    address: &str,
    module_id: &str,
    func: &str,
    ty_args: Vec<TypeTag>,
    args: Vec<Vec<u8>>,
    vm_status: &str,
) {
    let txn = account.sign_with_transaction_builder(context.transaction_factory().script_function(
        ScriptFunction::new(
            ModuleId::new(
                AccountAddress::from_hex_literal(address).unwrap(),
                Identifier::new(module_id).unwrap(),
            ),
            Identifier::new(func).unwrap(),
            ty_args,
            args,
        ),
    ));

    test_transaction_vm_status(context, txn, false, vm_status).await
}

async fn test_transaction_vm_status(
    context: TestContext,
    txn: SignedTransaction,
    success: bool,
    vm_status: &str,
) {
    let body = bcs::to_bytes(&txn).unwrap();
    // we don't validate transaction payload when submit txn into mempool.
    context
        .expect_status_code(202)
        .post_bcs_txn("/transactions", body)
        .await;

    context.commit_mempool_txns(1).await;

    let resp = context
        .get(format!("/transactions/{}", txn.committed_hash().to_hex_literal()).as_str())
        .await;
    assert_eq!(
        resp["success"].as_bool().unwrap(),
        success,
        "{}",
        pretty(&resp)
    );
    assert_eq!(resp["vm_status"].as_str().unwrap(), vm_status);
}

#[tokio::test]
async fn test_submit_transaction_rejects_payload_too_large_bcs_txn_body() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(413)
        .post_bcs_txn(
            "/transactions",
            gen_string(context.context.content_length_limit() + 1).as_bytes(),
        )
        .await;
    assert_json(
        resp,
        json!({
          "code": 413,
          "message": "The request payload is too large"
        }),
    );
}

#[tokio::test]
async fn test_submit_transaction_rejects_payload_too_large_json_body() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(413)
        .post(
            "/transactions",
            json!({
                "data": gen_string(context.context.content_length_limit()+1).as_bytes(),
            }),
        )
        .await;
    assert_json(
        resp,
        json!({
          "code": 413,
          "message": "The request payload is too large"
        }),
    );
}

#[tokio::test]
async fn test_submit_transaction_rejects_invalid_content_type() {
    let context = new_test_context();
    let req = warp::test::request()
        .header("content-type", "invalid")
        .method("POST")
        .body("text")
        .path("/transactions");

    let resp = context.expect_status_code(415).execute(req).await;
    assert_json(
        resp,
        json!({
            "code": 415,
            "message": "The request's content-type is not supported"
        }),
    );
}

#[tokio::test]
async fn test_submit_transaction_rejects_invalid_json() {
    let context = new_test_context();
    let req = warp::test::request()
        .header("content-type", "application/json")
        .method("POST")
        .body("invalid json")
        .path("/transactions");

    let resp = context.expect_status_code(400).execute(req).await;
    assert_json(
        resp,
        json!({
            "code": 400,
            "message": "Request body deserialize error: expected value at line 1 column 1"
        }),
    );
}

#[tokio::test]
async fn test_create_signing_message_rejects_payload_too_large_json_body() {
    let context = new_test_context();

    let resp = context
        .expect_status_code(413)
        .post(
            "/transactions/signing_message",
            json!({
                "data": gen_string(context.context.content_length_limit()+1).as_bytes(),
            }),
        )
        .await;
    assert_json(
        resp,
        json!({
          "code": 413,
          "message": "The request payload is too large"
        }),
    );
}

#[tokio::test]
async fn test_create_signing_message_rejects_invalid_content_type() {
    let context = new_test_context();
    let req = warp::test::request()
        .header("content-type", "invalid")
        .method("POST")
        .body("text")
        .path("/transactions/signing_message");

    let resp = context.expect_status_code(415).execute(req).await;
    assert_json(
        resp,
        json!({
            "code": 415,
            "message": "The request's content-type is not supported"
        }),
    );
}

#[tokio::test]
async fn test_create_signing_message_rejects_invalid_json() {
    let context = new_test_context();
    let req = warp::test::request()
        .header("content-type", "application/json")
        .method("POST")
        .body("invalid json")
        .path("/transactions/signing_message");

    let resp = context.expect_status_code(400).execute(req).await;
    assert_json(
        resp,
        json!({
            "code": 400,
            "message": "Request body deserialize error: expected value at line 1 column 1"
        }),
    );
}

#[tokio::test]
async fn test_create_signing_message_rejects_no_content_length_request() {
    let context = new_test_context();
    let req = warp::test::request()
        .header("content-type", "application/json")
        .method("POST")
        .path("/transactions/signing_message");

    let resp = context.expect_status_code(411).execute(req).await;
    assert_json(
        resp,
        json!({
            "code": 411,
            "message": "A content-length header is required"
        }),
    );
}

fn gen_string(len: u64) -> String {
    let mut rng = thread_rng();
    std::iter::repeat(())
        .map(|()| rng.sample(Alphanumeric))
        .take(len as usize)
        .map(char::from)
        .collect()
}
