import { DomainBase } from 'fulcrum';

export default class UserDomain extends DomainBase {
    constructor() {
        super('users'); // Just pass the domain name
    }

    /**
     * Handle user creation requests
     * Auto-mapped to 'user_create_request' messages
     */
    async userCreateHandler(userData, context) {
        console.log('Creating user:', userData);
        
        // Validate user data
        if (!userData.email || !userData.name) {
            throw new Error('Email and name are required');
        }
        
        // Create user in database
        const result = await this.createRecord('users', {
            name: userData.name,
            email: userData.email,
            created_at: new Date().toISOString()
        }, context.requestId);
        
        if (!result.success) {
            throw new Error(`Failed to create user: ${result.error}`);
        }

        // For create, we might need to query back the created user if the ID is auto-generated
        // For simplicity, let's assume the ID is returned or we can query it back.
        // For now, we'll return a success message.
        return { success: true, message: "User created successfully", id: result.lastInsertId };
    }

    /**
     * Handle user index/list requests
     * Auto-mapped to 'user_index_request' messages
     */
    async userIndexHandler(query, context) {
        console.log('Fetching users with query:', query);
        
        const users = await this.findRecords('users', {}, context.requestId);
        console.log('USERS')
        const data = JSON.parse(users.payload);
        console.log(data.data)
        return data.data;
    }

    /**
     * Handle user detail requests
     * Auto-mapped to 'user_show_request' messages
     */
    async userShowHandler(params, context) {
        console.log(params)
        console.log(context)

        const userId = params.user_id;
        
        if (!userId) {
            throw new Error('User ID is required');
        }

        // Find user by ID
        const users = await this.findRecords('users', { id: userId }, context.requestId);
        const data = JSON.parse(users.payload);
        
        if (data,data.length === 0) {
            throw new Error('User not found');
        }

        console.log(data)
        return data.data[0];
    }

    /**
     * Handle user update requests
     * Auto-mapped to 'user_update_request' messages
     */
    async userUpdateHandler(data, context) {
        const { id, ...updateData } = data;
        
        if (!id) {
            throw new Error('User ID is required');
        }

        const result = await this.updateRecord('users', id, updateData, context.requestId);
        
        if (!result.success) {
            throw new Error(`Failed to update user: ${result.error}`);
        }

        return { success: true, message: "User updated successfully", rowsAffected: result.rowsAffected };
    }

    /**
     * Optional lifecycle hook - called after domain starts
     */
    async onStart() {
        console.log('User domain is ready!');
        
        // Run any initialization code here
        // await this.loadConfiguration();
        // await this.setupCaches();
    }
}
