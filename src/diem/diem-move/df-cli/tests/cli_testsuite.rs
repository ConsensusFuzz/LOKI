// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use move_cli::sandbox::commands::test;

use std::path::{Path, PathBuf};

fn run_all(args_path: &Path) -> datatest_stable::Result<()> {
    test::run_one(
        args_path,
        &PathBuf::from("../../target/debug/df-cli"),
        /* use_temp_dir */ true,
        /* track_cov */ false,
    )?;
    Ok(())
}

// runs all the tests
datatest_stable::harness!(run_all, "tests/testsuite", r"args.txt$");
