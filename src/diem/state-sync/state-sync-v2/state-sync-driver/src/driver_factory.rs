// Copyright (c) The Diem Core Contributors
// SPDX-License-Identifier: Apache-2.0

use crate::{
    driver::{DriverConfiguration, StateSyncDriver},
    driver_client::{DriverClient, DriverNotification},
    notification_handlers::{
        BootstrapNotificationHandler, ConsensusNotificationHandler, MempoolNotificationHandler,
    },
    storage_synchronizer::StorageSynchronizer,
};
use consensus_notifications::ConsensusNotificationListener;
use data_streaming_service::streaming_client::StreamingServiceClient;
use diem_config::config::NodeConfig;
use diem_infallible::RwLock;
use diem_types::waypoint::Waypoint;
use event_notifications::EventSubscriptionService;
use executor_types::ChunkExecutor;
use futures::channel::mpsc;
use mempool_notifications::MempoolNotificationSender;
use std::{boxed::Box, sync::Arc};
use storage_interface::default_protocol::DbReaderWriter;
use tokio::runtime::{Builder, Runtime};

/// Creates a new state sync driver and client
pub struct DriverFactory {
    driver_runtime: Option<Runtime>,
    notification_sender: mpsc::UnboundedSender<DriverNotification>,
}

impl DriverFactory {
    /// Creates and spawns a new state sync driver
    pub fn create_and_spawn_driver<M: MempoolNotificationSender + 'static>(
        create_runtime: bool,
        node_config: &NodeConfig,
        waypoint: Waypoint,
        storage: DbReaderWriter,
        chunk_executor: Box<dyn ChunkExecutor>,
        mempool_notification_sender: M,
        consensus_listener: ConsensusNotificationListener,
        event_subscription_service: EventSubscriptionService,
        streaming_service_client: StreamingServiceClient,
    ) -> Self {
        // Create the notification handlers
        let (notification_sender, notification_receiver) = mpsc::unbounded();
        let bootstrap_notification_handler =
            BootstrapNotificationHandler::new(notification_receiver);
        let consensus_notification_handler = ConsensusNotificationHandler::new(consensus_listener);
        let mempool_notification_handler =
            MempoolNotificationHandler::new(mempool_notification_sender);

        // Create the driver configuration
        let driver_configuration = DriverConfiguration::new(node_config.base.role, waypoint);

        // Create a storage synchronizer
        let storage_synchronizer =
            StorageSynchronizer::new(chunk_executor, Arc::new(RwLock::new(storage)));

        // Create the driver
        let state_sync_driver = StateSyncDriver::new(
            bootstrap_notification_handler,
            consensus_notification_handler,
            driver_configuration,
            event_subscription_service,
            mempool_notification_handler,
            storage_synchronizer,
            streaming_service_client,
        );

        // Spawn the driver
        let driver_runtime = if create_runtime {
            // Create a new runtime for the driver
            let driver_runtime = Builder::new_multi_thread()
                .thread_name("state-sync-driver")
                .enable_all()
                .build()
                .expect("Failed to create state sync v2 driver runtime!");
            driver_runtime.spawn(state_sync_driver.start_driver());
            Some(driver_runtime)
        } else {
            // Spawn the driver on the current runtime
            tokio::spawn(state_sync_driver.start_driver());
            None
        };

        Self {
            driver_runtime,
            notification_sender,
        }
    }

    /// Returns a new client that can be used to communicate with the driver
    pub fn create_driver_client(&self) -> DriverClient {
        DriverClient::new(self.notification_sender.clone())
    }
}
