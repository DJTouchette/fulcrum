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
        const user = await this.createRecord('users', {
            name: userData.name,
            email: userData.email,
            created_at: new Date().toISOString()
        }, context.requestId);
        
        // Send welcome email
        await this.email(context.requestId)
            .to(userData.email)
            .subject('Welcome!')
            .template('welcome')
            .send();
        
        console.log('User created and welcome email sent');
        return user; // This becomes the response data
    }

    /**
     * Handle user index/list requests
     * Auto-mapped to 'user_index_request' messages
     */
    async userIndexHandler(query, context) {
        console.log('Fetching users with query:', query);
        
        // Return mock data for now - replace with actual DB query
        return [
            { id: 1, name: 'John Doe', email: 'john@example.com' },
            { id: 2, name: 'Jane Doe', email: 'jane@example.com' },
        ];
    }

    /**
     * Handle user detail requests
     * Auto-mapped to 'user_show_request' messages
     */
    async userShowHandler(params, context) {
        const userId = params.id;
        
        if (!userId) {
            throw new Error('User ID is required');
        }

        // Find user by ID
        const users = await this.findRecords('users', { id: userId }, context.requestId);
        
        if (users.length === 0) {
            throw new Error('User not found');
        }

        return users[0];
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

        const updatedUser = await this.updateRecord('users', id, updateData, context.requestId);
        return updatedUser;
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
