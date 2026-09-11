package mysqlstore

var schemaV1DDL = []string{
	`CREATE TABLE IF NOT EXISTS environments (
            id BINARY(16) NOT NULL PRIMARY KEY,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            archived_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
            UNIQUE KEY environments_resource_key (resource_key)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS configs (
            id BINARY(16) NOT NULL PRIMARY KEY,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            archived_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
            UNIQUE KEY configs_resource_key (resource_key)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS config_env_states (
            config_id BINARY(16) NOT NULL,
            environment_id BINARY(16) NOT NULL,
            current_revision BIGINT UNSIGNED NOT NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
            PRIMARY KEY (config_id, environment_id),
            KEY config_env_states_environment (environment_id, config_id),
            CONSTRAINT config_env_states_config_fk FOREIGN KEY (config_id) REFERENCES configs (id),
            CONSTRAINT config_env_states_environment_fk FOREIGN KEY (environment_id) REFERENCES environments (id),
            CONSTRAINT config_env_states_revision_positive CHECK (current_revision > 0)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS config_revisions (
            config_id BINARY(16) NOT NULL,
            environment_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
            format VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            content MEDIUMBLOB NOT NULL,
            content_sha256 BINARY(32) NOT NULL,
            operation_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            actor_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            actor_id VARCHAR(255) NOT NULL,
            source_config_id BINARY(16) NULL,
            source_environment_id BINARY(16) NULL,
            source_revision BIGINT UNSIGNED NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            PRIMARY KEY (config_id, environment_id, revision),
            KEY config_revisions_operation (operation_id),
            KEY config_revisions_created (created_at),
            CONSTRAINT config_revisions_config_fk FOREIGN KEY (config_id) REFERENCES configs (id),
            CONSTRAINT config_revisions_environment_fk FOREIGN KEY (environment_id) REFERENCES environments (id),
            CONSTRAINT config_revisions_format CHECK (format IN ('yaml', 'json')),
            CONSTRAINT config_revisions_size CHECK (OCTET_LENGTH(content) <= 5242880),
            CONSTRAINT config_revisions_revision_positive CHECK (revision > 0)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS config_revision_vault_refs (
            config_id BINARY(16) NOT NULL,
            environment_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
			namespace_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            item_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            field_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			PRIMARY KEY (config_id, environment_id, revision, namespace_key, item_key, field_key),
			KEY config_revision_vault_refs_item (namespace_key, item_key, field_key),
            CONSTRAINT config_revision_vault_refs_revision_fk
                FOREIGN KEY (config_id, environment_id, revision)
                REFERENCES config_revisions (config_id, environment_id, revision)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_items (
            id BINARY(16) NOT NULL PRIMARY KEY,
			namespace_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            current_revision BIGINT UNSIGNED NOT NULL,
            archived_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY vault_items_identity (namespace_key, resource_key),
            CONSTRAINT vault_items_revision_positive CHECK (current_revision > 0)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_fields (
            item_id BINARY(16) NOT NULL,
            id BINARY(16) NOT NULL,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            field_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            archived_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            PRIMARY KEY (item_id, id),
            UNIQUE KEY vault_fields_resource_key (item_id, resource_key),
            CONSTRAINT vault_fields_item_fk FOREIGN KEY (item_id) REFERENCES vault_items (id),
            CONSTRAINT vault_fields_type CHECK (field_type IN ('text', 'secret', 'file'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_item_revisions (
            item_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
            item_display_name VARCHAR(255) NOT NULL,
            structure_sha256 BINARY(32) NOT NULL,
            algorithm VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            key_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            nonce VARBINARY(32) NOT NULL,
            ciphertext MEDIUMBLOB NOT NULL,
            encrypted_dek VARBINARY(1024) NOT NULL,
            ciphertext_size INT UNSIGNED NOT NULL,
            operation_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            actor_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            actor_id VARCHAR(255) NOT NULL,
            restored_from_revision BIGINT UNSIGNED NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            PRIMARY KEY (item_id, revision),
            KEY vault_item_revisions_operation (operation_id),
            KEY vault_item_revisions_created (created_at),
            CONSTRAINT vault_item_revisions_item_fk FOREIGN KEY (item_id) REFERENCES vault_items (id),
            CONSTRAINT vault_item_revisions_revision_positive CHECK (revision > 0),
            CONSTRAINT vault_item_revisions_ciphertext_size CHECK (ciphertext_size = OCTET_LENGTH(ciphertext))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_revision_fields (
            item_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
            field_id BINARY(16) NOT NULL,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            field_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            archived BOOLEAN NOT NULL DEFAULT FALSE,
            PRIMARY KEY (item_id, revision, field_id),
            UNIQUE KEY vault_revision_fields_key (item_id, revision, resource_key),
            CONSTRAINT vault_revision_fields_revision_fk
                FOREIGN KEY (item_id, revision) REFERENCES vault_item_revisions (item_id, revision),
            CONSTRAINT vault_revision_fields_field_fk
                FOREIGN KEY (item_id, field_id) REFERENCES vault_fields (item_id, id),
            CONSTRAINT vault_revision_fields_type CHECK (field_type IN ('text', 'secret', 'file'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_revision_variants (
            item_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
            variant_id BINARY(16) NOT NULL,
            ordinal INT UNSIGNED NOT NULL,
            PRIMARY KEY (item_id, revision, variant_id),
            UNIQUE KEY vault_revision_variants_ordinal (item_id, revision, ordinal),
            CONSTRAINT vault_revision_variants_revision_fk
                FOREIGN KEY (item_id, revision) REFERENCES vault_item_revisions (item_id, revision)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS vault_revision_variant_environments (
            item_id BINARY(16) NOT NULL,
            revision BIGINT UNSIGNED NOT NULL,
            variant_id BINARY(16) NOT NULL,
            environment_id BINARY(16) NOT NULL,
            PRIMARY KEY (item_id, revision, environment_id),
            KEY vault_revision_variant_environments_variant (item_id, revision, variant_id),
            KEY vault_revision_variant_environments_environment (environment_id, item_id),
            CONSTRAINT vault_revision_variant_environments_variant_fk
                FOREIGN KEY (item_id, revision, variant_id)
                REFERENCES vault_revision_variants (item_id, revision, variant_id),
            CONSTRAINT vault_revision_variant_environments_environment_fk
                FOREIGN KEY (environment_id) REFERENCES environments (id)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS api_tokens (
            id BINARY(16) NOT NULL PRIMARY KEY,
            public_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            display_prefix VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            secret_digest BINARY(32) NOT NULL,
            allow_without_mtls BOOLEAN NOT NULL DEFAULT FALSE,
            expires_at DATETIME(6) NULL,
            revoked_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            UNIQUE KEY api_tokens_public_id (public_id),
            KEY api_tokens_active (revoked_at, expires_at)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS api_token_environments (
            token_id BINARY(16) NOT NULL,
            environment_id BINARY(16) NOT NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            PRIMARY KEY (token_id, environment_id),
            KEY api_token_environments_environment (environment_id, token_id),
            CONSTRAINT api_token_environments_token_fk FOREIGN KEY (token_id) REFERENCES api_tokens (id),
            CONSTRAINT api_token_environments_environment_fk FOREIGN KEY (environment_id) REFERENCES environments (id)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS client_certificates (
            id BINARY(16) NOT NULL PRIMARY KEY,
            fingerprint_sha256 BINARY(32) NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            certificate_der BLOB NOT NULL,
            subject VARCHAR(1024) NOT NULL,
            serial_hex VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            not_before DATETIME(6) NOT NULL,
            not_after DATETIME(6) NOT NULL,
            revoked_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            UNIQUE KEY client_certificates_fingerprint (fingerprint_sha256),
            KEY client_certificates_active (revoked_at, not_after)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS operations (
            operation_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
            request_sha256 BINARY(32) NOT NULL,
            actor_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            actor_id VARCHAR(255) NOT NULL,
            outcome VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            response_json JSON NULL,
            resulting_revision BIGINT UNSIGNED NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            completed_at DATETIME(6) NULL,
            KEY operations_created (created_at),
            CONSTRAINT operations_outcome CHECK (outcome IN ('pending', 'success', 'no_change', 'conflict', 'validation_failed'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS management_sessions (
            token CHAR(43) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
            data BLOB NOT NULL,
            expiry TIMESTAMP(6) NOT NULL,
            KEY management_sessions_expiry (expiry)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS notification_destinations (
            id BINARY(16) NOT NULL PRIMARY KEY,
            resource_key VARCHAR(63) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            display_name VARCHAR(255) NOT NULL,
            provider VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            safe_host VARCHAR(255) NOT NULL,
            masked_suffix VARCHAR(32) NOT NULL,
            algorithm VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            key_version VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            nonce VARBINARY(32) NOT NULL,
            ciphertext BLOB NOT NULL,
            encrypted_dek VARBINARY(1024) NOT NULL,
            enabled BOOLEAN NOT NULL DEFAULT TRUE,
            archived_at DATETIME(6) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
            UNIQUE KEY notification_destinations_resource_key (resource_key),
            CONSTRAINT notification_destinations_provider CHECK (provider IN ('generic_webhook', 'feishu_bot'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS notification_subscriptions (
            destination_id BINARY(16) NOT NULL,
            event_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            PRIMARY KEY (destination_id, event_type),
            CONSTRAINT notification_subscriptions_destination_fk
                FOREIGN KEY (destination_id) REFERENCES notification_destinations (id)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS outbox_events (
            id BINARY(16) NOT NULL PRIMARY KEY,
            kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            event_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            operation_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
            payload JSON NOT NULL,
            status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
            attempts INT UNSIGNED NOT NULL DEFAULT 0,
            next_attempt_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            claimed_at DATETIME(6) NULL,
            completed_at DATETIME(6) NULL,
            last_error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            KEY outbox_events_ready (kind, status, next_attempt_at),
            KEY outbox_events_operation (operation_id),
            CONSTRAINT outbox_events_operation_fk FOREIGN KEY (operation_id) REFERENCES operations (operation_id),
            CONSTRAINT outbox_events_kind CHECK (kind IN ('audit', 'notification')),
            CONSTRAINT outbox_events_status CHECK (status IN ('pending', 'processing', 'completed', 'dead'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS notification_targets (
            outbox_event_id BINARY(16) NOT NULL,
            destination_id BINARY(16) NOT NULL,
            status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
            attempts INT UNSIGNED NOT NULL DEFAULT 0,
            next_attempt_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            last_error_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
            PRIMARY KEY (outbox_event_id, destination_id),
            KEY notification_targets_ready (outbox_event_id, status, next_attempt_at),
            CONSTRAINT notification_targets_event_fk FOREIGN KEY (outbox_event_id) REFERENCES outbox_events (id),
            CONSTRAINT notification_targets_destination_fk FOREIGN KEY (destination_id) REFERENCES notification_destinations (id),
            CONSTRAINT notification_targets_status CHECK (status IN ('pending', 'succeeded', 'dead')),
            CONSTRAINT notification_targets_attempts CHECK (attempts <= 8)
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,

	`CREATE TABLE IF NOT EXISTS notification_deliveries (
            id BINARY(16) NOT NULL PRIMARY KEY,
            outbox_event_id BINARY(16) NOT NULL,
            destination_id BINARY(16) NOT NULL,
            attempt INT UNSIGNED NOT NULL,
            status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
            http_status SMALLINT UNSIGNED NULL,
            latency_ms INT UNSIGNED NULL,
            provider_error_code VARCHAR(128) NULL,
            created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
            UNIQUE KEY notification_deliveries_attempt (outbox_event_id, destination_id, attempt),
            KEY notification_deliveries_destination (destination_id, created_at),
            CONSTRAINT notification_deliveries_event_fk FOREIGN KEY (outbox_event_id) REFERENCES outbox_events (id),
            CONSTRAINT notification_deliveries_destination_fk FOREIGN KEY (destination_id) REFERENCES notification_destinations (id),
            CONSTRAINT notification_deliveries_status CHECK (status IN ('succeeded', 'retrying', 'dead'))
        ) ENGINE=InnoDB DEFAULT CHARACTER SET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`,
}
