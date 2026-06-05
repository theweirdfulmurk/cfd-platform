import { useState } from 'react';
import { CreateSimulation } from '../components/CreateSimulation';
import { SimulationList } from '../components/SimulationList';
import './HomePage.css';

export function HomePage() {
  const [refreshKey, setRefreshKey] = useState(0);

  return (
    <div className="app">
      <header className="topbar">
        <span className="brand">
          <span className="brand-mark" aria-hidden="true" />
          <span className="brand-name">Платформа инженерных расчётов</span>
        </span>
        <span className="cluster" title="Кластер доступен">
          <span className="cluster-dot" aria-hidden="true" />
          кластер активен
        </span>
      </header>

      <main className="workspace">
        <aside className="pane pane-form">
          <CreateSimulation onCreated={() => setRefreshKey(k => k + 1)} />
        </aside>
        <section className="pane pane-list">
          <SimulationList key={refreshKey} />
        </section>
      </main>
    </div>
  );
}
