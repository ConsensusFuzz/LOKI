// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{pool::Entry, SYMBOL_POOL};
use serde::{Deserialize, Serialize};
use std::{cmp::Ordering, fmt, num::NonZeroU64};

/// Represents a string that has been cached.
///
/// A `Symbol` represents a pointer to string data that is owned by the global
/// symbol pool; it is not the string data itself. This enables this
/// representation to implement `Copy` and other traits that some string types
/// cannot.
///
/// The strings that `Symbol` types represent are added to the global cache as
/// the `Symbol` are created.
///
/// ```
///# use crate::move_symbol_pool::Symbol;
/// let s1 = Symbol::from("hi"); // "hi" is stored in the global cache
/// let s2 = Symbol::from("hi"); // "hi" is already stored, cache does not grow
/// assert_eq!(s1, s2);
/// ```
///
/// Use the method [`as_str()`] to access the string value that a `Symbol`
/// represents. `Symbol` also implements the [`Display`] trait, so it can be
/// printed as an ordinary string would. This makes it easier to use with
/// crates that print strings to a terminal, such as codespan.
///
/// ```
///# use crate::move_symbol_pool::Symbol;
/// let message = format!("{} {}",
///     Symbol::from("hello").as_str(),
///     Symbol::from("world"));
/// assert_eq!(message, "hello world");
/// ```
///
/// [`as_str()`]: crate::Symbol::as_str
/// [`Display`]: std::fmt::Display
#[derive(Clone, Copy, Debug, Deserialize, Eq, Hash, PartialEq, Serialize)]
pub struct Symbol(NonZeroU64);

impl Symbol {
    pub fn as_str(&self) -> &str {
        let ptr = self.0.get() as *const Entry;
        let entry = unsafe { &*ptr };
        &entry.string
    }
}

impl From<&str> for Symbol {
    fn from(s: &str) -> Self {
        let mut pool = SYMBOL_POOL.lock().expect("could not acquire lock on pool");
        let address = pool.insert(s).as_ptr() as u64;
        Symbol(NonZeroU64::new(address).expect("address of symbol cannot be null"))
    }
}

impl From<String> for Symbol {
    fn from(s: String) -> Self {
        Self::from(s.as_str())
    }
}

impl AsRef<str> for Symbol {
    fn as_ref(&self) -> &str {
        self.as_str()
    }
}

impl fmt::Display for Symbol {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        self.as_str().fmt(f)
    }
}

impl Ord for Symbol {
    fn cmp(&self, other: &Symbol) -> Ordering {
        if self.0 == other.0 {
            Ordering::Equal
        } else {
            self.as_str().cmp(other.as_str())
        }
    }
}

impl PartialOrd for Symbol {
    fn partial_cmp(&self, other: &Symbol) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

#[cfg(test)]
mod tests {
    use crate::Symbol;
    use std::mem::size_of;

    #[test]
    fn test_size() {
        // Assert that the size of a Symbol is fairly small. Since it'll be used
        // throughout the Move codebase, increases to this size should be
        // scrutinized.
        assert_eq!(size_of::<Symbol>(), size_of::<u64>());
    }

    #[test]
    fn test_from_different_strings_have_different_addresses() {
        let s1 = Symbol::from("hi");
        let s2 = Symbol::from("hello");
        assert_ne!(s1.0, s2.0);
    }

    #[test]
    fn test_from_identical_strings_have_the_same_address() {
        let s1 = Symbol::from("bonjour");
        let s2 = Symbol::from("bonjour");
        assert_eq!(s1.0, s2.0);
    }
}
