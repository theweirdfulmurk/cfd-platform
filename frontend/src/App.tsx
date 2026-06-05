import { HomePage } from './pages/HomePage';
import { ToastProvider } from './components/ToastProvider';

function App() {
  return (
    <ToastProvider>
      <HomePage />
    </ToastProvider>
  );
}

export default App;