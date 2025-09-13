module.exports = async function(context) {
  console.log(context);
  const user = await context.fulcrum.db.find('users', { id: 1 });

  console.log(user)
  // const response = await fetch('https://bible-api.com/data/web/random')
  // const data = await response.json();

  return [];
};
