// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::param::{Param, TransactionVersionParam};

use diem_api_types::{Error, TransactionId};

use anyhow::Result;
use serde::Deserialize;
use std::num::NonZeroU16;

const DEFAULT_PAGE_SIZE: u16 = 25;
const MAX_PAGE_SIZE: u16 = 1000;

#[derive(Clone, Debug, Deserialize)]
pub(crate) struct Page {
    start: Option<TransactionVersionParam>,
    limit: Option<Param<NonZeroU16>>,
}

impl Page {
    pub fn start(&self, default: u64, max: u64) -> Result<u64, Error> {
        let version = self
            .start
            .clone()
            .map(|v| v.parse("start"))
            .unwrap_or_else(|| Ok(default))?;
        if version > max {
            return Err(Error::not_found(
                "transaction",
                TransactionId::Version(version),
                max,
            ));
        }
        Ok(version)
    }

    pub fn limit(&self) -> Result<u16, Error> {
        let limit = self
            .limit
            .clone()
            .map(|v| v.parse("limit"))
            .unwrap_or_else(|| Ok(NonZeroU16::new(DEFAULT_PAGE_SIZE).unwrap()))?
            .get();
        if limit > MAX_PAGE_SIZE {
            return Err(Error::invalid_param(
                "limit",
                format!("{}, exceed limit {}", limit, MAX_PAGE_SIZE),
            ));
        }
        Ok(limit)
    }
}
