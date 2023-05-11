// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    client::Client,
    error::{Error, Result, WaitForTransactionError},
    request::MethodRequest,
    response::{MethodResponse, Response},
    retry::Retry,
    verifying_client::{methods::VerifyingBatch, state_store::StateStore},
};
use diem_crypto::hash::{CryptoHash, HashValue};
use diem_json_rpc_types::views::{
    AccountView, CurrencyInfoView, EventView, MetadataView, TransactionView,
};
use diem_types::{
    account_address::AccountAddress,
    event::EventKey,
    transaction::{SignedTransaction, Transaction, Version},
    trusted_state::TrustedState,
    waypoint::Waypoint,
};
use std::{fmt::Debug, time::Duration};

// TODO(philiphayes): fill out rest of the methods
// TODO(philiphayes): all clients should validate chain id (allow users to trust-on-first-use or pre-configure)
// TODO(philiphayes): we could abstract the async client so VerifyingClient takes a dyn Trait?

/// The `VerifyingClient` is a [Diem JSON-RPC client] that verifies Diem's
/// cryptographic proofs when it makes API calls.
///
/// ## Concurrency
///
/// When issuing multiple concurrent requests with the `VerifyingClient`, we guarantee:
///
/// 1. Each response batch is fulfilled and verified at a ledger version that
///    is greater than or equal to the current trusted version _at the time we
///    made the request batch_, though not necessarily the globally most
///    recent trusted ledger version.
///
/// 2. Requests made serially within a single thread of execution appear
///    strictly ordered, i.e., they were fulfilled and verified at
///    monotonically increasing ledger versions (`v1 <= v2 <= ...`).
///
/// Consequently, without any other effort, multiple concurrent requests may have
/// responses that appear inconsistent or out-of-order. For example, naively
/// making concurrent `get_account(..)` requests will (most likely) show accounts
/// at different ledger versions; even further, the first response you receive may
/// show a more recent ledger version than the second response.
///
/// To avoid this issue, users should pin a concurrent batch of requests to the
/// same ledger version if they want to avoid an inconsistent ledger view.
///
/// [Diem JSON-RPC client]: https://github.com/diem/diem/blob/master/json-rpc/json-rpc-spec.md
#[derive(Clone, Debug)]
pub struct VerifyingClient<S> {
    inner: Client,
    state_store: S,
    retry: Retry,
}

impl<S: StateStore> VerifyingClient<S> {
    pub fn new(mut inner: Client, state_store: S) -> Result<Self> {
        let retry = inner.take_retry();
        let this = Self {
            inner,
            state_store,
            retry,
        };
        this.version()?; // state store must be initialized
        Ok(this)
    }

    pub fn new_with_state(
        mut inner: Client,
        state_store: S,
        initial_state: &TrustedState,
    ) -> Result<Self> {
        let retry = inner.take_retry();
        let this = Self {
            inner,
            state_store,
            retry,
        };
        this.ratchet_state(Some(initial_state))?;
        Ok(this)
    }

    /// Get a snapshot of our current trusted ledger [`Version`].
    pub fn version(&self) -> Result<Version> {
        self.state_store
            .latest_state_version()
            .map_err(Error::state_store)?
            .ok_or_else(|| Error::unknown("state store is empty; probably not initialized yet"))
    }

    /// Get a snapshot of our current trusted [`Waypoint`].
    pub fn waypoint(&self) -> Result<Waypoint> {
        Ok(self.trusted_state()?.waypoint())
    }

    /// Get a snapshot of our current [`TrustedState`].
    pub fn trusted_state(&self) -> Result<TrustedState> {
        self.state_store
            .latest_state()
            .map_err(Error::state_store)?
            .ok_or_else(|| Error::unknown("state store is empty; probably not initialized yet"))
    }

    pub async fn wait_for_signed_transaction(
        &self,
        txn: &SignedTransaction,
        timeout: Option<Duration>,
        delay: Option<Duration>,
    ) -> Result<Response<TransactionView>, WaitForTransactionError> {
        let response = self
            .wait_for_transaction(
                txn.sender(),
                txn.sequence_number(),
                txn.expiration_timestamp_secs(),
                Transaction::UserTransaction(txn.clone()).hash(),
                timeout,
                delay,
            )
            .await?;

        if !response.inner().vm_status.is_executed() {
            return Err(WaitForTransactionError::TransactionExecutionFailed(
                response.into_inner(),
            ));
        }

        Ok(response)
    }

    pub async fn wait_for_transaction(
        &self,
        address: AccountAddress,
        seq: u64,
        expiration_time_secs: u64,
        txn_hash: HashValue,
        timeout: Option<Duration>,
        delay: Option<Duration>,
    ) -> Result<Response<TransactionView>, WaitForTransactionError> {
        const DEFAULT_TIMEOUT: Duration = Duration::from_secs(5);
        const DEFAULT_DELAY: Duration = Duration::from_millis(50);

        let start = std::time::Instant::now();
        while start.elapsed() < timeout.unwrap_or(DEFAULT_TIMEOUT) {
            let txn_resp = self
                .get_account_transaction(address, seq, true)
                .await
                .map_err(WaitForTransactionError::GetTransactionError)?;

            let (maybe_txn, state) = txn_resp.into_parts();

            if let Some(txn) = maybe_txn {
                if txn.hash != txn_hash {
                    return Err(WaitForTransactionError::TransactionHashMismatchError(txn));
                }

                return Ok(Response::new(txn, state));
            }

            if expiration_time_secs <= state.timestamp_usecs / 1_000_000 {
                return Err(WaitForTransactionError::TransactionExpired);
            }

            tokio::time::sleep(delay.unwrap_or(DEFAULT_DELAY)).await;
        }

        Err(WaitForTransactionError::Timeout)
    }

    /// Issue `get_state_proof` requests until we successfully sync to the remote
    /// node's current version (unless we experience a verification error or other
    /// I/O error).
    pub async fn sync(&self) -> Result<()> {
        self.retry
            .retry_async(|| async {
                while self.sync_one_step().await? {
                    // TODO(philiphayes): logging / callback?
                }
                Ok(())
            })
            .await
    }

    /// Issue a single `get_state_proof` request and try to verify it. Returns
    /// `Ok(true)` if, after verification, we still need to sync more. Returns
    /// `Ok(false)` if we have finished syncing.
    pub async fn sync_one_step(&self) -> Result<bool> {
        // batch([]) is effectively just a get_state_proof request
        match self.batch_without_retry(vec![]).await {
            Ok(_) => Ok(false),
            Err(err) => {
                if err.is_need_sync() {
                    Ok(true)
                } else {
                    Err(err)
                }
            }
        }
    }

    /// Try to compare-and-swap a verified trusted state change into the state store.
    /// If the client is issuing muiltiple concurrent requests, the potential
    /// new trusted state might not be newer than the current trusted state,
    /// in which case we just don't persist this change.
    pub fn ratchet_state(&self, new_state: Option<&TrustedState>) -> Result<()> {
        if let Some(new_state) = new_state {
            self.state_store
                .store(new_state)
                .map_err(Error::state_store)?;
        }
        Ok(())
    }

    /// Submit a new signed user transaction.
    ///
    /// Note: we don't verify anything about the server's response here. If the
    /// server is behaving maliciously, they can claim our transaction is
    /// malformed when it is not, they can broadcast our valid transaction but
    /// tell us it is too old, or they can accept our invalid transaction without
    /// giving us any indication that it's bad.
    ///
    /// Unfortunately, there's nothing for us to verify that their response is
    /// correct or not; the only way to get around this is by broadcasting our
    /// transaction to multiple different servers. As long as one is honest, our
    /// valid transaction will eventually be committed. This client handles a
    /// connection to a single server, so the broadcasting needs to happen at a
    /// higher layer.
    pub async fn submit(&self, txn: &SignedTransaction) -> Result<Response<()>> {
        // TODO(philiphayes): fix retries for txn submit. need to
        // avoid the whole verifying client machinery for submits
        self.inner.submit(txn).await
    }

    pub async fn get_metadata_by_version(
        &self,
        version: Version,
    ) -> Result<Response<MetadataView>> {
        self.request(MethodRequest::get_metadata_by_version(version))
            .await?
            .and_then(MethodResponse::try_into_get_metadata)
    }

    pub async fn get_metadata(&self) -> Result<Response<MetadataView>> {
        self.request(MethodRequest::get_metadata())
            .await?
            .and_then(MethodResponse::try_into_get_metadata)
    }

    pub async fn get_account(
        &self,
        address: AccountAddress,
    ) -> Result<Response<Option<AccountView>>> {
        self.request(MethodRequest::get_account(address))
            .await?
            .and_then(MethodResponse::try_into_get_account)
    }

    pub async fn get_account_by_version(
        &self,
        address: AccountAddress,
        version: Version,
    ) -> Result<Response<Option<AccountView>>> {
        self.request(MethodRequest::get_account_by_version(address, version))
            .await?
            .and_then(MethodResponse::try_into_get_account)
    }

    pub async fn get_transactions(
        &self,
        start_version: Version,
        limit: u64,
        include_events: bool,
    ) -> Result<Response<Vec<TransactionView>>> {
        self.request(MethodRequest::get_transactions(
            start_version,
            limit,
            include_events,
        ))
        .await?
        .and_then(MethodResponse::try_into_get_transactions)
    }

    pub async fn get_account_transaction(
        &self,
        address: AccountAddress,
        seq_num: u64,
        include_events: bool,
    ) -> Result<Response<Option<TransactionView>>> {
        self.request(MethodRequest::get_account_transaction(
            address,
            seq_num,
            include_events,
        ))
        .await?
        .and_then(MethodResponse::try_into_get_account_transaction)
    }

    pub async fn get_account_transactions(
        &self,
        address: AccountAddress,
        start_seq_num: u64,
        limit: u64,
        include_events: bool,
    ) -> Result<Response<Vec<TransactionView>>> {
        self.request(MethodRequest::get_account_transactions(
            address,
            start_seq_num,
            limit,
            include_events,
        ))
        .await?
        .and_then(MethodResponse::try_into_get_account_transactions)
    }

    pub async fn get_events(
        &self,
        key: EventKey,
        start_seq: u64,
        limit: u64,
    ) -> Result<Response<Vec<EventView>>> {
        self.request(MethodRequest::get_events(key, start_seq, limit))
            .await?
            .and_then(MethodResponse::try_into_get_events)
    }

    pub async fn get_currencies(&self) -> Result<Response<Vec<CurrencyInfoView>>> {
        self.request(MethodRequest::get_currencies())
            .await?
            .and_then(MethodResponse::try_into_get_currencies)
    }

    pub async fn get_network_status(&self) -> Result<Response<u64>> {
        self.request(MethodRequest::get_network_status())
            .await?
            .and_then(MethodResponse::try_into_get_network_status)
    }

    pub fn actual_batch_size(&self, requests: &[MethodRequest]) -> Result<usize> {
        Ok(VerifyingBatch::from_batch(requests.to_vec()).num_requests(&self.trusted_state()?))
    }

    /// Send a single request via `VerifyingClient::batch`.
    pub async fn request(&self, request: MethodRequest) -> Result<Response<MethodResponse>> {
        self.retry
            .retry_async(|| self.request_without_retry(request.clone()))
            .await
    }

    pub async fn batch(
        &self,
        requests: Vec<MethodRequest>,
    ) -> Result<Vec<Result<Response<MethodResponse>>>> {
        self.retry
            .retry_async(|| self.batch_without_retry(requests.clone()))
            .await
    }

    //
    // Private Helpers
    //

    async fn request_without_retry(
        &self,
        request: MethodRequest,
    ) -> Result<Response<MethodResponse>> {
        let mut responses = self.batch_without_retry(vec![request]).await?.into_iter();
        responses
            .next()
            .expect("batch guarantees the correct number of responses")
    }

    async fn batch_without_retry(
        &self,
        requests: Vec<MethodRequest>,
    ) -> Result<Vec<Result<Response<MethodResponse>>>> {
        let request_trusted_state = self.trusted_state()?;

        // transform each request into verifying sub-request batches
        let batch = VerifyingBatch::from_batch(requests);
        // flatten and collect sub-request batches into flat list of requests
        let requests = batch.collect_requests(&request_trusted_state);
        // actually send the batch
        let responses = self.inner.batch(requests).await?;
        // validate responses and state proof w.r.t. request trusted state
        let (new_state, maybe_responses) =
            batch.verify_responses(&request_trusted_state, responses)?;
        // try to ratchet our trusted state in our state store
        self.ratchet_state(new_state.as_ref())?;

        let responses = maybe_responses
            .ok_or_else(|| Error::need_sync("too far behind server, need to sync more"))?;
        Ok(responses)
    }
}
