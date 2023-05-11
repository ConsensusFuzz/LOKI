// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use anyhow::{bail, format_err};
use diem_temppath::TempPath;
use move_command_line_common::files::MOVE_EXTENSION;
use std::{
    fs, io,
    io::Write,
    path::{Path, PathBuf},
    process::Command,
};

use diem_sdk::{
    client::{views::AmountView, BlockingClient, WaitForTransactionError},
    transaction_builder::{Currency, TransactionFactory},
    types::{
        account_address::AccountAddress,
        transaction::{ModuleBundle, Script, TransactionArgument, TransactionPayload},
        LocalAccount,
    },
};
use forge::{AdminContext, AdminTest, Result, Test};
use move_ir_compiler::Compiler;

pub struct MalformedScript;

impl Test for MalformedScript {
    fn name(&self) -> &'static str {
        "smoke-test::malformed-script"
    }
}

impl AdminTest for MalformedScript {
    fn run<'t>(&self, ctx: &mut AdminContext<'t>) -> Result<()> {
        let client = ctx.client();
        let transaction_factory = ctx.chain_info().transaction_factory();
        enable_open_publishing(
            &client,
            &transaction_factory,
            &mut ctx.chain_info().root_account,
        )?;
        let mut account = ctx.random_account();
        ctx.chain_info()
            .create_parent_vasp_account(Currency::XUS, account.authentication_key())?;
        ctx.chain_info()
            .fund(Currency::XUS, account.address(), 100)?;

        let script_path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../..")
            .join("testsuite/smoke-test/src/dev_modules/test_script.move")
            .canonicalize()?;
        let move_stdlib_dir = move_stdlib::move_stdlib_modules_full_path();
        let diem_core_framework_dir = diem_framework::diem_core_modules_full_path();
        let diem_payment_framework_dir = diem_framework::diem_payment_modules_full_path();
        let dependencies = &[
            move_stdlib_dir.as_str(),
            diem_core_framework_dir.as_str(),
            diem_payment_framework_dir.as_str(),
        ];
        let compiled_script = compile_program(script_path.to_str().unwrap(), dependencies)?;

        // the script expects two arguments. Passing only one in the test, which will cause a failure.
        let client = ctx.client();
        let factory = ctx.chain_info().transaction_factory();
        let txn =
            account.sign_with_transaction_builder(factory.payload(TransactionPayload::Script(
                Script::new(compiled_script, vec![], vec![TransactionArgument::U64(10)]),
            )));
        client.submit(&txn)?;
        assert!(matches!(
            client
                .wait_for_signed_transaction(&txn, None, None)
                .unwrap_err(),
            WaitForTransactionError::TransactionExecutionFailed(..)
        ));

        // Previous transaction should not choke the system.
        ctx.chain_info()
            .fund(Currency::XUS, account.address(), 100)?;

        Ok(())
    }
}

pub struct ExecuteCustomModuleAndScript;

impl Test for ExecuteCustomModuleAndScript {
    fn name(&self) -> &'static str {
        "smoke-test::execute-custom-module-and-script"
    }
}

impl AdminTest for ExecuteCustomModuleAndScript {
    fn run<'t>(&self, ctx: &mut AdminContext<'t>) -> Result<()> {
        let client = ctx.client();
        let factory = ctx.chain_info().transaction_factory();
        enable_open_publishing(&client, &factory, &mut ctx.chain_info().root_account)?;

        let mut account1 = ctx.random_account();
        ctx.chain_info()
            .create_parent_vasp_account(Currency::XUS, account1.authentication_key())?;
        ctx.chain_info()
            .fund(Currency::XUS, account1.address(), 100)?;

        assert_eq!(
            vec![AmountView {
                amount: 100,
                currency: "XUS".to_string()
            }],
            client
                .get_account(account1.address())?
                .into_inner()
                .unwrap()
                .balances
        );

        let account2 = ctx.random_account();
        ctx.chain_info()
            .create_parent_vasp_account(Currency::XUS, account2.authentication_key())?;
        ctx.chain_info()
            .fund(Currency::XUS, account2.address(), 1)?;

        // Get the path to the Move stdlib sources
        let move_stdlib_dir = move_stdlib::move_stdlib_modules_full_path();
        let diem_core_framework_dir = diem_framework::diem_core_modules_full_path();
        let diem_payment_framework_dir = diem_framework::diem_payment_modules_full_path();

        // Make a copy of module.move with "{{sender}}" substituted.
        let module_path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../..")
            .join("testsuite/smoke-test/src/dev_modules/module.move")
            .canonicalize()?;
        let copied_module_path =
            copy_file_with_sender_address(&module_path, account1.address()).unwrap();
        let unwrapped_module_path = copied_module_path.to_str().unwrap();

        let compiled_module = compile_program(
            unwrapped_module_path,
            &[
                move_stdlib_dir.as_str(),
                diem_core_framework_dir.as_str(),
                diem_payment_framework_dir.as_str(),
            ],
        )?;

        let publish_txn = account1.sign_with_transaction_builder(factory.payload(
            TransactionPayload::ModuleBundle(ModuleBundle::singleton(compiled_module)),
        ));
        client.submit(&publish_txn)?;
        client.wait_for_signed_transaction(&publish_txn, None, None)?;

        // Make a copy of script.move with "{{sender}}" substituted.
        let script_path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../..")
            .join("testsuite/smoke-test/src/dev_modules/script.move")
            .canonicalize()?;
        let copied_script_path =
            copy_file_with_sender_address(&script_path, account1.address()).unwrap();
        let unwrapped_script_path = copied_script_path.to_str().unwrap();

        let compiled_script = compile_program(
            unwrapped_script_path,
            &[
                unwrapped_module_path,
                move_stdlib_dir.as_str(),
                diem_core_framework_dir.as_str(),
                diem_payment_framework_dir.as_str(),
            ],
        )?;

        let execute_txn = account1.sign_with_transaction_builder(factory.payload(
            TransactionPayload::Script(Script::new(
                compiled_script,
                vec![],
                vec![
                    TransactionArgument::Address(account2.address()),
                    TransactionArgument::U64(10),
                ],
            )),
        ));
        client.submit(&execute_txn)?;
        client.wait_for_signed_transaction(&execute_txn, None, None)?;

        assert_eq!(
            vec![AmountView {
                amount: 90,
                currency: "XUS".to_string()
            }],
            client
                .get_account(account1.address())?
                .into_inner()
                .unwrap()
                .balances
        );

        assert_eq!(
            vec![AmountView {
                amount: 11,
                currency: "XUS".to_string()
            }],
            client
                .get_account(account2.address())?
                .into_inner()
                .unwrap()
                .balances
        );

        Ok(())
    }
}

fn copy_file_with_sender_address(file_path: &Path, sender: AccountAddress) -> io::Result<PathBuf> {
    let tmp_source_path = TempPath::new().as_ref().with_extension(MOVE_EXTENSION);
    let mut tmp_source_file = std::fs::File::create(tmp_source_path.clone())?;
    let mut code = fs::read_to_string(file_path)?;
    code = code.replace("{{sender}}", &format!("0x{}", sender));
    writeln!(tmp_source_file, "{}", code)?;
    Ok(tmp_source_path)
}

pub fn enable_open_publishing(
    client: &BlockingClient,
    transaction_factory: &TransactionFactory,
    root_account: &mut LocalAccount,
) -> Result<()> {
    let script_body = {
        let code = "
            import 0x1.DiemTransactionPublishingOption;

            main(account: signer) {
                DiemTransactionPublishingOption.set_open_script(&account);
                DiemTransactionPublishingOption.set_open_module(&account, true);

                return;
            }
        ";

        let compiler = Compiler {
            deps: diem_framework_releases::current_modules().iter().collect(),
        };
        compiler.into_script_blob(code).expect("Failed to compile")
    };

    let txn = root_account.sign_with_transaction_builder(transaction_factory.payload(
        TransactionPayload::Script(Script::new(script_body, vec![], vec![])),
    ));
    client.submit(&txn)?;
    client.wait_for_signed_transaction(&txn, None, None)?;
    Ok(())
}

/// Compile Move program
pub fn compile_program(file_path: &str, dependency_paths: &[&str]) -> Result<Vec<u8>> {
    let tmp_output_dir = TempPath::new();
    tmp_output_dir
        .create_as_dir()
        .expect("error creating temporary output directory");
    let tmp_output_path = tmp_output_dir.as_ref().display().to_string();

    let mut command = Command::new("cargo");
    command
        .args(&["run", "-p", "move-lang", "--bin", "move-build", "--"])
        .arg(file_path)
        .args(&["-o", &tmp_output_path]);

    for dep in dependency_paths {
        command.args(&["-d", dep]);
    }
    for (name, addr) in diem_framework::diem_framework_named_addresses() {
        command.args(&["-a", &format!("{}={:#X}", name, addr)]);
    }

    let output = command.output()?;
    if !output.status.success() {
        return Err(format_err!("compilation failed"));
    }

    let mut output_files = walkdir::WalkDir::new(tmp_output_path)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            let path = e.path();
            e.file_type().is_file()
                && path
                    .extension()
                    .and_then(|s| s.to_str())
                    .map(|ext| ext == "mv")
                    .unwrap_or(false)
        })
        .filter_map(|e| e.path().to_str().map(|s| s.to_string()))
        .collect::<Vec<_>>();
    if output_files.is_empty() {
        bail!("compiler failed to produce an output file")
    }

    let compiled_program = if output_files.len() != 1 {
        bail!("compiler output has more than one file")
    } else {
        fs::read(output_files.pop().unwrap())?
    };

    Ok(compiled_program)
}
