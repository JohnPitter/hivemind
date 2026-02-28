import { useState, useRef, useEffect } from 'react';
import { Send, Trash2, Cpu } from 'lucide-react';
import type { ChatMessage } from '../types';
import { mockStreamResponse } from '../lib/mock-data';

interface ChatPlaygroundProps {
  modelId: string;
}

export function ChatPlayground({ modelId }: ChatPlaygroundProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async () => {
    if (!input.trim() || isStreaming) return;

    const userMsg: ChatMessage = { role: 'user', content: input.trim() };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setIsStreaming(true);

    // Start streaming mock response
    const words = mockStreamResponse(input);
    const assistantMsg: ChatMessage = { role: 'assistant', content: '' };
    setMessages((prev) => [...prev, assistantMsg]);

    for (let i = 0; i < words.length; i++) {
      await new Promise((r) => setTimeout(r, 40 + Math.random() * 80));
      setMessages((prev) => {
        const updated = [...prev];
        const last = updated[updated.length - 1];
        updated[updated.length - 1] = {
          ...last,
          content: last.content + (i > 0 ? ' ' : '') + words[i],
        };
        return updated;
      });
    }

    setIsStreaming(false);
  };

  const handleClear = () => {
    setMessages([]);
  };

  return (
    <div className="flex flex-col h-full">
      {/* Model info */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <Cpu className="w-4 h-4 text-amber" />
          <span className="text-sm font-medium text-text-primary">{modelId.split('/').pop()}</span>
          <span className="text-xs text-text-muted">via distributed inference</span>
        </div>
        {messages.length > 0 && (
          <button
            onClick={handleClear}
            className="flex items-center gap-1.5 text-xs text-text-muted hover:text-red transition-colors"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Clear
          </button>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto space-y-4 mb-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <Cpu className="w-12 h-12 text-amber/20 mb-4" />
            <h3 className="text-lg font-medium text-text-secondary mb-2">
              Chat with {modelId.split('/').pop()}
            </h3>
            <p className="text-sm text-text-muted max-w-md">
              Your messages are processed through distributed tensor parallelism
              across all peers in the room.
            </p>
          </div>
        )}

        {messages.map((msg, i) => (
          <MessageBubble key={i} message={msg} />
        ))}

        {isStreaming && (
          <div className="flex items-center gap-2 text-xs text-text-muted pl-4">
            <span className="w-1.5 h-1.5 rounded-full bg-amber animate-pulse" />
            Generating via 3-node tensor parallelism...
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="flex gap-3">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && !e.shiftKey && handleSend()}
          placeholder="Type a message..."
          disabled={isStreaming}
          className="flex-1 bg-bg-secondary border border-border rounded-xl px-4 py-3 text-sm text-text-primary placeholder-text-muted focus:outline-none focus:border-amber/50 transition-colors disabled:opacity-50"
        />
        <button
          onClick={handleSend}
          disabled={!input.trim() || isStreaming}
          className="px-4 py-3 bg-amber text-black rounded-xl font-medium text-sm hover:bg-amber-light transition-colors disabled:opacity-30 disabled:cursor-not-allowed flex items-center gap-2"
        >
          <Send className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === 'user';

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`
          max-w-[80%] rounded-2xl px-4 py-3 text-sm leading-relaxed
          ${isUser
            ? 'bg-amber text-black rounded-br-md'
            : 'bg-bg-tertiary text-text-primary border border-border rounded-bl-md'
          }
        `}
      >
        {message.content}
      </div>
    </div>
  );
}
