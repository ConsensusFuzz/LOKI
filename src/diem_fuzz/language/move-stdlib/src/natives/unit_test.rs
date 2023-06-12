// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use move_binary_format::errors::PartialVMResult;
use move_core_types::gas_schedule::ONE_GAS_UNIT;
use move_vm_runtime::native_functions::NativeContext;
use move_vm_types::{
    loaded_data::runtime_types::Type, natives::function::NativeResult, pop_arg, values::Value,
};
use smallvec::smallvec;
use std::collections::VecDeque;

use move_core_types::account_address::AccountAddress;

pub fn native_create_signers_for_testing(
    _context: &mut NativeContext,
    ty_args: Vec<Type>,
    mut args: VecDeque<Value>,
) -> PartialVMResult<NativeResult> {
    debug_assert!(ty_args.is_empty());
    debug_assert!(args.len() == 1);

    let num_signers = pop_arg!(args, u64);
    let signers = Value::vector_for_testing_only(
        (0..num_signers).map(|i| Value::signer(AccountAddress::new((i as u128).to_le_bytes()))),
    );

    Ok(NativeResult::ok(ONE_GAS_UNIT, smallvec![signers]))
}
