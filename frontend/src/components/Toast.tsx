import { useEffect, useState } from 'react';
import { IconCheck, IconAlert } from './icons';
import './Toast.css';

interface ToastProps {
  message: string;
  type: 'success' | 'error' | 'info';
  onClose: () => void;
}

export function Toast({ message, type, onClose }: ToastProps) {
  const [isClosing, setIsClosing] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      setIsClosing(true);
      setTimeout(onClose, 300);
    }, 3500);
    return () => clearTimeout(timer);
  }, [onClose]);

  const handleClose = () => {
    setIsClosing(true);
    setTimeout(onClose, 300);
  };

  return (
    <div className={`toast toast-${type}${isClosing ? ' is-closing' : ''}`} role="status">
      <span className="toast-icon" aria-hidden="true">
        {type === 'success' ? <IconCheck size={15} /> : <IconAlert size={15} />}
      </span>
      <span className="toast-msg">{message}</span>
      <button className="toast-close" onClick={handleClose} aria-label="Dismiss">
        <svg width="13" height="13" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M3 3l6 6M9 3l-6 6" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        </svg>
      </button>
    </div>
  );
}
