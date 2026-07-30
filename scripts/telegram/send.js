const { loadConfig } = require('./config');

async function send(text) {
  const { botToken, chatId } = loadConfig();
  const res = await fetch(`https://api.telegram.org/bot${botToken}/sendMessage`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ chat_id: chatId, text }),
  });
  const data = await res.json();
  if (!data.ok) {
    console.error('Telegram sendMessage failed:', JSON.stringify(data));
    process.exit(1);
  }
}

const text = process.argv.slice(2).join(' ');
if (!text) {
  console.error('Usage: node send.js "message text"');
  process.exit(1);
}
send(text).catch(err => {
  console.error('Error sending message:', err.message);
  process.exit(1);
});
