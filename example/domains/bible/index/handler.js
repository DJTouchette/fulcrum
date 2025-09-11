module.exports = async function(context) {
  const response = await fetch('https://bible-api.com/data/web/random')
  const data = await response.json();
  return data;
};
