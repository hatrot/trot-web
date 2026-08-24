(function () {
  'use strict';

  // State
  let isOpen = false;
  let isSending = false;
  let chatHistory = [];

  // Create Widget DOM
  function createWidget() {
    // Check if already created
    if (document.getElementById('trotChatWindow')) return;

    const widget = document.createElement('div');
    widget.className = 'trot-chat-widget';
    widget.innerHTML = `
      <!-- Floating Action Button (Optional fallback) -->
      <button class="trot-chat-trigger" id="trotChatTrigger" aria-label="AI Q&Aを開く" title="AI Q&A（AIアシスタント）">
        <svg viewBox="0 0 24 24">
          <path d="M12 2C6.48 2 2 6.48 2 12c0 1.85.5 3.58 1.38 5.07L2 22l5.07-1.38C8.42 21.5 10.15 22 12 22c5.52 0 10-4.48 10-10S17.52 2 12 2zm1 14h-2v-2h2v2zm0-4h-2V7h2v5z"/>
        </svg>
      </button>

      <!-- Chat Window -->
      <div class="trot-chat-window" id="trotChatWindow">
        <!-- Header -->
        <div class="trot-chat-header">
          <div class="trot-chat-header-title">
            <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
              <path d="M12 2a2 2 0 0 1 2 2c0 .74-.4 1.39-1 1.73V7h1a7 7 0 0 1 7 7h1a1 1 0 0 1 1 1v3a1 1 0 0 1-1 1h-1v1a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-1H2a1 1 0 0 1-1-1v-3a1 1 0 0 1 1-1h1a7 7 0 0 1 7-7h1V5.73c-.6-.34-1-.99-1-1.73a2 2 0 0 1 2-2M7.5 13A2.5 2.5 0 0 0 5 15.5 2.5 2.5 0 0 0 7.5 18a2.5 2.5 0 0 0 2.5-2.5A2.5 2.5 0 0 0 7.5 13m9 0a2.5 2.5 0 0 0-2.5 2.5 2.5 2.5 0 0 0 2.5 2.5 2.5 2.5 0 0 0 2.5-2.5 2.5 2.5 0 0 0-2.5-2.5z"/>
            </svg>
            <h4>AIアシスタント</h4>
            <span class="trot-chat-badge" id="trotChatBadge">AI Assistant</span>
          </div>
          <button class="trot-chat-close" id="trotChatClose" aria-label="閉じる">&times;</button>
        </div>

        <!-- Body / Messages -->
        <div class="trot-chat-body" id="trotChatBody">
          <div class="trot-msg trot-msg-bot">
            こんにちは！エイチ・エィ・トロットのAIアシスタントです。<br><br>
            当社の開発実績や技術スタック、お仕事のご相談など、何でもお気軽にお尋ねください。
          </div>
        </div>

        <!-- Suggestion Chips -->
        <div class="trot-chat-chips" id="trotChatChips">
          <button class="trot-chip-btn" data-q="トロットの30年の歴史を教えて！">📜 沿革・歴史</button>
          <button class="trot-chip-btn" data-q="得意な技術スタックや開発分野は？">💻 得意技術</button>
          <button class="trot-chip-btn" data-q="NACK5TOUCHなどの実績について教えて">📻 プロダクト実績</button>
          <button class="trot-chip-btn" data-q="連絡先や公式SNSリンクは？">🌐 連絡先・リンク</button>
        </div>

        <!-- Footer / Input -->
        <div class="trot-chat-footer">
          <input type="text" class="trot-chat-input" id="trotChatInput" placeholder="質問を入力してください..." />
          <button class="trot-chat-send" id="trotChatSend" aria-label="送信">
            <svg viewBox="0 0 24 24">
              <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
            </svg>
          </button>
        </div>
      </div>
    `;

    document.body.appendChild(widget);
    attachEvents();
  }

  // Format markdown links & line breaks safely
  function formatReply(text) {
    const div = document.createElement('div');
    div.textContent = text;
    let safe = div.innerHTML;

    // Convert markdown links [text](url)
    safe = safe.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+|mailto:[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');

    // Convert bold **text**
    safe = safe.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

    // Convert newlines
    safe = safe.replace(/\n/g, '<br>');

    return safe;
  }

  // Attach Event Listeners
  function attachEvents() {
    const trigger = document.getElementById('trotChatTrigger');
    const closeBtn = document.getElementById('trotChatClose');
    const win = document.getElementById('trotChatWindow');
    const input = document.getElementById('trotChatInput');
    const sendBtn = document.getElementById('trotChatSend');
    const chips = document.getElementById('trotChatChips');

    function toggleChat(open) {
      isOpen = (open !== undefined) ? open : !isOpen;
      if (isOpen) {
        win.classList.add('open');
        input.focus();
      } else {
        win.classList.remove('open');
      }
    }

    // Bind in-page open buttons (e.g. #open-ai-qa-btn)
    const inPageButtons = document.querySelectorAll('#open-ai-qa-btn, [data-open-ai-qa], .open-ai-qa-trigger');
    inPageButtons.forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        toggleChat(true);
      });
    });

    if (trigger) {
      trigger.addEventListener('click', () => toggleChat());
    }

    if (closeBtn) {
      closeBtn.addEventListener('click', () => toggleChat(false));
    }

    if (sendBtn) {
      sendBtn.addEventListener('click', () => {
        const q = input.value.trim();
        if (q) sendMessage(q);
      });
    }

    let isComposing = false;

    if (input) {
      input.addEventListener('compositionstart', () => {
        isComposing = true;
      });

      input.addEventListener('compositionend', () => {
        isComposing = false;
      });

      input.addEventListener('keydown', (e) => {
        // IME変換中のEnter（確定操作）では送信しない
        if (e.isComposing || isComposing || e.keyCode === 229) {
          return;
        }

        // 確定後のEnter単体で送信
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          const q = input.value.trim();
          if (q) sendMessage(q);
        }
      });
    }

    if (chips) {
      chips.addEventListener('click', (e) => {
        const btn = e.target.closest('.trot-chip-btn');
        if (btn && btn.dataset.q) {
          sendMessage(btn.dataset.q);
        }
      });
    }

    // Expose global helper
    window.openTrotChat = function (initialQuestion) {
      toggleChat(true);
      if (initialQuestion) {
        sendMessage(initialQuestion);
      }
    };
  }

  // Add Message to UI
  function appendMessage(sender, htmlContent) {
    const body = document.getElementById('trotChatBody');
    const msg = document.createElement('div');
    msg.className = `trot-msg trot-msg-${sender}`;
    msg.innerHTML = htmlContent;
    body.appendChild(msg);
    body.scrollTop = body.scrollHeight;
    return msg;
  }

  // Show Typing Indicator
  function showTyping() {
    const body = document.getElementById('trotChatBody');
    const typing = document.createElement('div');
    typing.className = 'trot-msg trot-msg-bot trot-typing';
    typing.id = 'trotTypingIndicator';
    typing.innerHTML = `
      <div class="trot-typing-dot"></div>
      <div class="trot-typing-dot"></div>
      <div class="trot-typing-dot"></div>
    `;
    body.appendChild(typing);
    body.scrollTop = body.scrollHeight;
  }

  function removeTyping() {
    const typing = document.getElementById('trotTypingIndicator');
    if (typing) typing.remove();
  }

  // Send Message to API
  async function sendMessage(question) {
    if (isSending) return;
    isSending = true;

    const input = document.getElementById('trotChatInput');
    const sendBtn = document.getElementById('trotChatSend');
    const badge = document.getElementById('trotChatBadge');

    input.value = '';
    sendBtn.disabled = true;

    // Display user message
    appendMessage('user', formatReply(question));

    // Show typing
    showTyping();

    try {
      const payload = {
        question: question,
        history: chatHistory.slice(-6) // Keep last 3 turns of context
      };

      const res = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      removeTyping();

      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || `HTTP ${res.status}`);
      }

      const data = await res.json();
      
      if (data.provider) {
        badge.textContent = data.provider;
      }

      if (data.reply) {
        let replyText = data.reply;

        // Check for contact submission tag: [[SUBMIT_CONTACT: {...}]]
        const contactMatch = replyText.match(/\[\[SUBMIT_CONTACT:\s*({.+?})\s*\]\]/s);
        if (contactMatch) {
          try {
            const contactData = JSON.parse(contactMatch[1]);
            // Clean display text
            replyText = replyText.replace(contactMatch[0], '').trim();
            
            // Send to /api/contact in background
            fetch('/api/contact', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                name: contactData.name || 'Web訪問者',
                email: contactData.email || '',
                content: contactData.content || '',
                source: 'AIアシスタント (Web Q&A)'
              })
            }).then(r => r.json()).then(res => {
              if (res.status === 'ok') {
                console.log('Contact inquiry notified to Slack:', res);
              }
            }).catch(e => console.error('Contact notification error:', e));

          } catch (jsonErr) {
            console.error('Failed to parse contact json:', jsonErr);
          }
        }

        appendMessage('bot', formatReply(replyText));

        // If bot is asking for confirmation to submit, show quick action chip
        if (replyText.includes('送信してよろしい') || replyText.includes('送信しますか')) {
          const chips = document.getElementById('trotChatChips');
          if (chips) {
            chips.innerHTML = `
              <button class="trot-chip-btn" style="background:#047857; color:#fff; border-color:#047857;" data-q="はい、この内容でサポートへ送信してください">📤 はい、送信してください</button>
              <button class="trot-chip-btn" data-q="内容を少し修正したいです">✏️ 修正したい</button>
            `;
          }
        }

        // Update history
        chatHistory.push({ role: 'user', content: question });
        chatHistory.push({ role: 'model', content: data.reply });
      } else {
        appendMessage('bot', '回答の取得に失敗しました。');
      }

    } catch (err) {
      removeTyping();
      appendMessage('bot', `⚠️ エラーが発生しました: ${err.message}`);
    } finally {
      isSending = false;
      sendBtn.disabled = false;
      input.focus();
    }
  }

  // Initialize on DOM Ready
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', createWidget);
  } else {
    createWidget();
  }
})();
