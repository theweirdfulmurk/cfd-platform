import { createContext, useContext, useState, useCallback, ReactNode } from 'react';
import { Toast } from './Toast';

type ToastType = 'success' | 'error' | 'info';
interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
}
interface ToastCtx {
  notify: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastCtx | null>(null);

export function useToast(): ToastCtx {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const remove = useCallback((id: number) => {
    setItems(list => list.filter(t => t.id !== id));
  }, []);

  const notify = useCallback((message: string, type: ToastType = 'info') => {
    const id = nextId++;
    setItems(list => [...list, { id, message, type }]);
  }, []);

  return (
    <ToastContext.Provider value={{ notify }}>
      {children}
      <div className="toast-stack">
        {items.map(t => (
          <Toast key={t.id} message={t.message} type={t.type} onClose={() => remove(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}
