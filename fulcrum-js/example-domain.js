const { DomainClient } = require('./index');

class UserDomain {
    constructor() {
        this.client = new DomainClient('user');
        this.setupHandlers();
    }

    setupHandlers() {
        // Handle incoming requests from the framework
        this.client.on('user_create_request', async (message) => {
            try {
                const userData = JSON.parse(message.payload);
                const user = await this.createUser(userData);
                
                // Send success response
                await this.client.sendMessage('user_create_response', {
                    success: true,
                    user: user
                });
            } catch (error) {
                await this.client.sendMessage('user_create_response', {
                    success: false,
                    error: error.message
                });
            }
        });
    }

    async createUser(userData) {
        console.log('Creating user:', userData);
        
        // Validate user data
        if (!userData.email || !userData.name) {
            throw new Error('Email and name are required');
        }

        try {
            // Create user in database
            const user = await this.client.createRecord('users', {
                name: userData.name,
                email: userData.email,
                created_at: new Date().toISOString()
            });

            console.log('User created:', user);

            // Send welcome email
            await this.client.email()
                .to(userData.email)
                .subject('Welcome!')
                .template('welcome')
                .send();

            console.log('Welcome email sent');
            return user;
        } catch (error) {
            console.error('Error creating user:', error);
            throw error;
        }
    }

    async start() {
        console.log('Starting user domain...');
        await this.client.connect();
        console.log('User domain started and connected to framework');
        
        // Test some operations after connecting
        setTimeout(async () => {
            await this.testOperations();
        }, 2000);
    }

    async testOperations() {
        console.log('\n--- Testing Domain Operations ---');
        
        try {
            // Test user creation
            console.log('Testing user creation...');
            await this.createUser({
                name: 'Test User',
                email: 'test@example.com'
            });
            
            // Test direct database operations
            console.log('Testing direct database operations...');
            const users = await this.client.findRecords('users', { name: 'Test User' });
            console.log('Found users:', users);
            
        } catch (error) {
            console.error('Test operations failed:', error);
        }
    }
}

// Start the domain if this file is run directly
if (require.main === module) {
    const userDomain = new UserDomain();
    userDomain.start().catch(console.error);
    
    // Graceful shutdown
    process.on('SIGINT', () => {
        console.log('\nShutting down...');
        userDomain.client.disconnect();
        process.exit(0);
    });
}
