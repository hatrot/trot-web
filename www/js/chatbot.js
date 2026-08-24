/**
 * TROT.CO.JP AI CHATBOT ENGINE (BILINGUAL EN/JP)
 */

document.addEventListener('DOMContentLoaded', () => {
  const triggerBtn = document.getElementById('chatbot-trigger');
  const chatWindow = document.getElementById('chatbot-window');
  const closeBtn = document.getElementById('chat-close-btn');
  const chatMessages = document.getElementById('chat-messages');
  const chatInput = document.getElementById('chat-input');
  const sendBtn = document.getElementById('chat-send-btn');

  if (!triggerBtn || !chatWindow) return;

  // Toggle Chat Window
  triggerBtn.addEventListener('click', () => {
    chatWindow.classList.toggle('active');
    if (chatWindow.classList.contains('active')) {
      chatInput.focus();
    }
  });

  closeBtn.addEventListener('click', () => {
    chatWindow.classList.remove('active');
  });

  // Knowledge Base (Bilingual)
  const knowledgeBase = [
    {
      keywords: ['history', 'journey', 'founded', '1993', '1995', '1996', '歴史', '沿革', '創業'],
      response: `H.A.Trot Co.,Ltd. was founded in October 1993 in Yokohama!<br>
• <b>Oct 1995</b>: Acquired Class C IP & launched <b>trot.co.jp</b><br>
• <b>Nov 1996</b>: Incorporated as a limited company (Game OS, financial systems, multimedia)<br>
• <b>2014-2016</b>: Interactive radio broadcasting apps (FM NACK5, STV Radio)`
    },
    {
      keywords: ['product', 'app', 'service', 'raditas', 'nack5', 'work', '製品', '事例', '実績'],
      response: `Our key featured products:<br>
• <b>RADITAS</b>: Reaction & feedback collection app for general radio stations.<br>
• <b>NACK5VOICE</b>: Real-time survey app for FM NACK5.<br>
• <b>NACK5TOUCH</b>: Live polling system reflecting listener sentiment in real time.`
    },
    {
      keywords: ['vision', 'philosophy', 'concept', 'dream', 'ai', '理念', 'ミッション'],
      response: `Our Vision:<br>
<i>"Your life with computer, We designed & build."</i><br>
<i>"Future AI imagines, We deliver it."</i><br>
Crafting next-generation digital experiences since 1993.`
    },
    {
      keywords: ['contact', 'email', 'location', 'yokohama', 'japan', '問い合わせ', '連絡', '場所'],
      response: `You can reach out to us at:<br>
• Location: Yokohama, Japan<br>
• Email: <a href="mailto:info@trot.co.jp" style="color:#38bdf8">info@trot.co.jp</a> or via the Contact section below.`
    }
  ];

  // Send Message Event
  const sendMessage = (text = null) => {
    const userText = text || chatInput.value.trim();
    if (!userText) return;

    // Render User Message
    appendMessage(userText, 'user');
    if (!text) chatInput.value = '';

    // Show Typing Indicator
    showTypingIndicator();

    // Generate Response
    setTimeout(() => {
      removeTypingIndicator();
      const botResponse = generateBotResponse(userText);
      appendMessage(botResponse.text, 'bot', botResponse.chips);
    }, 750);
  };

  sendBtn.addEventListener('click', () => sendMessage());
  chatInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') sendMessage();
  });

  // Append Message to Chat Log
  function appendMessage(content, sender, chips = null) {
    const msgDiv = document.createElement('div');
    msgDiv.className = `chat-msg ${sender}`;
    msgDiv.innerHTML = content;

    if (chips && chips.length > 0) {
      const chipsContainer = document.createElement('div');
      chipsContainer.className = 'quick-chips';
      chips.forEach(chipText => {
        const chipBtn = document.createElement('button');
        chipBtn.className = 'chip-btn';
        chipBtn.textContent = chipText;
        chipBtn.addEventListener('click', () => sendMessage(chipText));
        chipsContainer.appendChild(chipBtn);
      });
      msgDiv.appendChild(chipsContainer);
    }

    chatMessages.appendChild(msgDiv);
    chatMessages.scrollTop = chatMessages.scrollHeight;
  }

  // Typing Indicator
  function showTypingIndicator() {
    const typingDiv = document.createElement('div');
    typingDiv.className = 'chat-msg bot typing-container';
    typingDiv.id = 'typing-indicator';
    typingDiv.innerHTML = `
      <div class="typing-dots">
        <div class="typing-dot"></div>
        <div class="typing-dot"></div>
        <div class="typing-dot"></div>
      </div>
    `;
    chatMessages.appendChild(typingDiv);
    chatMessages.scrollTop = chatMessages.scrollHeight;
  }

  function removeTypingIndicator() {
    const typingIndicator = document.getElementById('typing-indicator');
    if (typingIndicator) typingIndicator.remove();
  }

  // Response Generator
  function generateBotResponse(input) {
    const cleanInput = input.toLowerCase();

    for (const kb of knowledgeBase) {
      if (kb.keywords.some(kw => cleanInput.includes(kw))) {
        return { text: kb.response };
      }
    }

    return {
      text: `Thank you for your message! Please select a topic below or type keywords like "History", "Products", or "Contact". (日本語でのご質問にもお答えします！)`,
      chips: ['Our History', 'Products & Apps', 'Contact Us']
    };
  }
});
