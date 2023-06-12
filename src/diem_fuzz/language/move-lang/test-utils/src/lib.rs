// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use move_command_line_common::files::{MOVE_EXTENSION, MOVE_IR_EXTENSION};
use std::path::{Path, PathBuf};
pub struct StringError(String);

pub const SENDER: &str = "0x8675309";

// These paths are relative to the move-lang directory.
pub const FUNCTIONAL_TEST_DIR: &str = "functional-tests/tests";
pub const MOVE_CHECK_DIR: &str = "tests/move_check";
pub const PATH_TO_IR_TESTS: &str = "../ir-testsuite/tests";

pub const MIGRATION_SUB_DIR: &str = "translated_ir_tests";
pub const TODO_EXTENSION: &str = "move_TODO";

pub const DEBUG_MODULE_FILE_NAME: &str = "debug.move";

pub const COMPLETED_DIRECTORIES: &[&str; 5] = &[
    "move/borrow_tests",
    "move/commands",
    "move/generics/instantiation_loops",
    "move/signer",
    "move/operators",
];

//**************************************************************************************************
// IR Test Translation
//**************************************************************************************************

pub fn ir_tests() -> impl Iterator<Item = (String, String)> {
    macro_rules! comp_to_string {
        ($comp_opt:expr) => {{
            $comp_opt.as_os_str().to_str()?
        }};
    }
    let num_root_components = Path::new(PATH_TO_IR_TESTS)
        .canonicalize()
        .unwrap()
        .components()
        .map(|_| 1)
        .sum();
    iterate_directory(Path::new(PATH_TO_IR_TESTS)).flat_map(move |path| {
        if path.extension()?.to_str()? != MOVE_IR_EXTENSION {
            return None;
        }
        let pathbuf = path.canonicalize().ok()?;
        let mut components = pathbuf.components();
        // skip over the components pointing to the IR test dir
        for _ in 0..num_root_components {
            components.next();
        }
        // iterate over the components starting with the file name
        let mut components = components.rev();
        let name = comp_to_string!(components.next().unwrap()).to_owned();
        // Combine the other components into one single string
        // These components represet the subdir path under the IR test directory. For migration
        // purposes, consider all of these as a single subdir
        let mut dir = String::new();
        for comp in components {
            let sep = if dir.is_empty() { "" } else { "/" };
            dir = format!("{}{}{}", comp_to_string!(comp), sep, dir)
        }
        Some((dir, name))
    })
}

pub fn translated_ir_test_name(has_main: bool, subdir: &str, name: &str) -> Option<String> {
    let fmt = |dir, migration_subdir, subdir, basename, ext| match migration_subdir {
        Some(migration_subdir) => format!(
            "{}/{}/{}/{}.{}",
            dir, migration_subdir, subdir, basename, ext
        ),
        None => format!("{}/{}/{}.{}", dir, subdir, basename, ext),
    };
    let check = |x| Path::new(x).is_file();
    let ft = fmt(FUNCTIONAL_TEST_DIR, None, subdir, name, MOVE_EXTENSION);
    let ft_todo = fmt(FUNCTIONAL_TEST_DIR, None, subdir, name, TODO_EXTENSION);
    let mc = fmt(
        MOVE_CHECK_DIR,
        Some(MIGRATION_SUB_DIR),
        subdir,
        name,
        MOVE_EXTENSION,
    );
    let mc_todo = fmt(
        MOVE_CHECK_DIR,
        Some(MIGRATION_SUB_DIR),
        subdir,
        name,
        TODO_EXTENSION,
    );
    if check(&ft) || check(&ft_todo) || check(&mc) || check(&mc_todo) {
        None
    } else if has_main {
        Some(ft)
    } else {
        Some(mc)
    }
}

fn iterate_directory(path: &Path) -> impl Iterator<Item = PathBuf> {
    walkdir::WalkDir::new(path)
        .into_iter()
        .map(::std::result::Result::unwrap)
        .filter(|entry| {
            entry.file_type().is_file()
                && entry
                    .file_name()
                    .to_str()
                    .map_or(false, |s| !s.starts_with('.')) // Skip hidden files
        })
        .map(|entry| entry.path().to_path_buf())
}
