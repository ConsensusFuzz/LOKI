// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use anyhow::{anyhow, Result};
use std::process::Command;

fn check_that_version_control_has_no_unstaged_changes() -> Result<()> {
    let output = Command::new("git")
        .arg("status")
        .arg("--porcelain")
        .output()
        .unwrap();
    if !(output.stdout.is_empty() && output.status.success()) {
        Err(anyhow!(
            "Git repository should be in a clean state, but found:\n{}\n",
            std::str::from_utf8(&output.stdout).unwrap_or("<binary>"),
        ))
    } else {
        Ok(())
    }
}

#[test]
fn test_that_generated_file_are_up_to_date_in_git() {
    // Don't check if git isn't in a clean state
    if check_that_version_control_has_no_unstaged_changes().is_ok() {
        // If this assertion fails, run the following command to re-generate DPN release:
        // `cargo run --release -p diem-framework`
        assert!(Command::new("cargo")
            .current_dir(std::env!("CARGO_MANIFEST_DIR"))
            .arg("run")
            .arg("--release")
            .arg("-p")
            .arg("diem-framework")
            .status()
            .unwrap()
            .success());

        // If this assertion fails, run the following command to re-generate experimental release:
        // `cargo run --release -p diem-framework -- --package experimental`
        assert!(Command::new("cargo")
            .current_dir(std::env!("CARGO_MANIFEST_DIR"))
            .arg("run")
            .arg("--release")
            .arg("-p")
            .arg("diem-framework")
            .arg("--")
            .arg("--package")
            .arg("experimental")
            .status()
            .unwrap()
            .success());

        // Running the stdlib tool should not create unstaged changes.
        check_that_version_control_has_no_unstaged_changes()
            .unwrap_or_else(|err_msg| panic!("{}", err_msg))
    }
}
