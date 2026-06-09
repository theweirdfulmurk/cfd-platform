import { useEffect, ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { IconX } from './icons';
import './Modal.css';

interface ModalProps {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
}

/** A glass dialog over a blurred backdrop. Closes on Escape or backdrop click,
 *  locks body scroll while open. */
export function Modal({ title, onClose, children }: ModalProps) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [onClose]);

  // Portal to <body>: a glassy ancestor (backdrop-filter) or any transformed
  // ancestor becomes the containing block for our position:fixed backdrop,
  // which pins it to that panel instead of the viewport — with a long list the
  // dialog then drifts far down the page. Rendering at the body root escapes
  // every such ancestor so the backdrop truly covers the viewport and centres.
  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        onClick={e => e.stopPropagation()}
      >
        <div className="modal-head">
          <div className="modal-title">{title}</div>
          <button className="modal-close" onClick={onClose} aria-label="Закрыть">
            <IconX size={16} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>,
    document.body,
  );
}
