module.exports = async function(context) {
  console.log('CONTEXT')
  console.log(context)

  const response = await fetch('https://bible-api.com/data/web/random')
  const data = await response.json();
  console.log('DATA')
  return data;
};
