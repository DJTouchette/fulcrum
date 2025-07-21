import UserDomain from './domains/users/domain.js';

async function main() {
  await new UserDomain().start();
}

await main()
