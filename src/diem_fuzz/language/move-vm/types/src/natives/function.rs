// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

//! Native Function Support
//!
//! All Move native functions have the following signature:
//!
//! `pub fn native_function(
//!     context: &mut impl NativeContext,
//!     ty_args: Vec<Type>,
//!     mut arguments: VecDeque<Value>,
//! ) -> PartialVMResult<NativeResult>;`
//!
//! arguments are passed with first argument at position 0 and so forth.
//! Popping values from `arguments` gives the aguments in reverse order (last first).
//! This module contains the declarations and utilities to implement a native
//! function.

use crate::{gas_schedule::NativeCostIndex, values::Value};
use move_core_types::gas_schedule::{
    AbstractMemorySize, CostTable, GasAlgebra, GasCarrier, InternalGasUnits,
};
use smallvec::SmallVec;

pub use move_binary_format::errors::PartialVMError;
pub use move_core_types::vm_status::StatusCode;

/// Result of a native function execution requires charges for execution cost.
///
/// An execution that causes an invariant violation would not return a `NativeResult` but
/// return a `PartialVMError` error directly.
/// All native functions must return a `PartialVMResult<NativeResult>` where an `Err` is returned
/// when an error condition is met that should not charge for the execution. A common example
/// is a VM invariant violation which should have been forbidden by the verifier.
/// Errors (typically user errors and aborts) that are logically part of the function execution
/// must be expressed in a `NativeResult` with a cost and a VMStatus.
pub struct NativeResult {
    /// The cost for running that function, whether successfully or not.
    pub cost: InternalGasUnits<GasCarrier>,
    /// Result of execution. This is either the return values or the error to report.
    pub result: Result<SmallVec<[Value; 1]>, u64>,
}

impl NativeResult {
    /// Return values of a successful execution.
    pub fn ok(cost: InternalGasUnits<GasCarrier>, values: SmallVec<[Value; 1]>) -> Self {
        NativeResult {
            cost,
            result: Ok(values),
        }
    }

    /// Failed execution. The failure is a runtime failure in the function and not an invariant
    /// failure of the VM which would raise a `PartialVMError` error directly.
    /// The only thing the funciton can specify is its abort code, as if it had invoked the `Abort`
    /// bytecode instruction
    pub fn err(cost: InternalGasUnits<GasCarrier>, abort_code: u64) -> Self {
        NativeResult {
            cost,
            result: Err(abort_code),
        }
    }
}

/// Return the native gas entry in `CostTable` for the given key.
/// The key is the specific native function index known to `CostTable`.
pub fn native_gas(
    table: &CostTable,
    key: NativeCostIndex,
    size: usize,
) -> InternalGasUnits<GasCarrier> {
    let gas_amt = table.native_cost(key as u8);
    let memory_size = AbstractMemorySize::new(std::cmp::max(1, size) as GasCarrier);
    debug_assert!(memory_size.get() > 0);
    gas_amt.total().mul(memory_size)
}

/// Return the argument at the top of the stack.
///
/// Arguments are passed to a native as a stack with first arg at the bottom of the stack.
/// Calling this API can help in making the code more readable.
/// It's good practice to pop all arguments in locals of the native function on function entry.
#[macro_export]
macro_rules! pop_arg {
    ($arguments:ident, $t:ty) => {{
        use $crate::natives::function::{NativeResult, PartialVMError, StatusCode};
        match $arguments.pop_back().map(|v| v.value_as::<$t>()) {
            None => {
                return Err(PartialVMError::new(
                    StatusCode::UNKNOWN_INVARIANT_VIOLATION_ERROR,
                ))
            }
            Some(Err(e)) => return Err(e),
            Some(Ok(v)) => v,
        }
    }};
}
