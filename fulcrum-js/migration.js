// Migration utilities for TypeScript/JavaScript domains

export class MigrationRunner {
    constructor(domainClient) {
        this.client = domainClient;
    }

    /**
     * Request migration execution via gRPC
     * This sends a migration request to the Go runtime
     */
    async runMigrations() {
        console.log('🔄 Requesting migrations from Fulcrum runtime...');
        
        try {
            const response = await this.client.sendMessage('migration_up_request', {
                domain: this.client.domainName,
                timestamp: new Date().toISOString()
            }, true);
            
            if (response.success) {
                console.log('✅ Migrations completed successfully');
                return JSON.parse(response.payload);
            } else {
                console.error('❌ Migration failed:', response.error);
                throw new Error(response.error);
            }
        } catch (error) {
            console.error('❌ Failed to run migrations:', error.message);
            throw error;
        }
    }

    /**
     * Get migration status via gRPC
     */
    async getStatus() {
        console.log('📋 Checking migration status...');
        
        try {
            const response = await this.client.sendMessage('migration_status_request', {
                domain: this.client.domainName
            }, true);
            
            if (response.success) {
                return JSON.parse(response.payload);
            } else {
                throw new Error(response.error);
            }
        } catch (error) {
            console.error('❌ Failed to get migration status:', error.message);
            throw error;
        }
    }

    /**
     * Rollback migrations via gRPC
     */
    async rollback(toVersion = null) {
        console.log(`⬇️  Requesting migration rollback${toVersion ? ` to version ${toVersion}` : ''}...`);
        
        try {
            const payload = {
                domain: this.client.domainName
            };
            
            if (toVersion !== null) {
                payload.to_version = toVersion;
            }
            
            const response = await this.client.sendMessage('migration_down_request', payload, true);
            
            if (response.success) {
                console.log('✅ Rollback completed successfully');
                return JSON.parse(response.payload);
            } else {
                console.error('❌ Rollback failed:', response.error);
                throw new Error(response.error);
            }
        } catch (error) {
            console.error('❌ Failed to rollback migrations:', error.message);
            throw error;
        }
    }
}

/**
 * Migration helper methods for domain base class
 */
export const MigrationMixin = {
    /**
     * Get migration runner for this domain
     */
    get migrations() {
        if (!this._migrationRunner) {
            this._migrationRunner = new MigrationRunner(this.client);
        }
        return this._migrationRunner;
    },

    /**
     * Run pending migrations for this domain
     */
    async runMigrations() {
        return await this.migrations.runMigrations();
    },

    /**
     * Get migration status for this domain
     */
    async getMigrationStatus() {
        return await this.migrations.getStatus();
    },

    /**
     * Rollback migrations for this domain
     */
    async rollbackMigrations(toVersion = null) {
        return await this.migrations.rollback(toVersion);
    },

    /**
     * Check if domain has pending migrations
     */
    async hasPendingMigrations() {
        try {
            const status = await this.getMigrationStatus();
            return status.pending_migrations && status.pending_migrations.length > 0;
        } catch (error) {
            console.error('Failed to check pending migrations:', error);
            return false;
        }
    },

    /**
     * Auto-run migrations on domain startup (optional)
     */
    async autoMigrate() {
        if (process.env.FULCRUM_AUTO_MIGRATE === 'true') {
            console.log('🔄 Auto-migration enabled, checking for pending migrations...');
            
            if (await this.hasPendingMigrations()) {
                console.log('📋 Found pending migrations, running them...');
                await this.runMigrations();
            } else {
                console.log('✅ No pending migrations found');
            }
        }
    }
};

export default { MigrationRunner, MigrationMixin };
