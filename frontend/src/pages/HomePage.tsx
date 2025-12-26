import { useState } from 'react';
import { CreateSimulation } from '../components/CreateSimulation';
import { SimulationList } from '../components/SimulationList';
import './HomePage.css';

export function HomePage() {
  const [refreshKey, setRefreshKey] = useState(0);

  return (
    <div className="home-page">
      <header>
        <h1>CFD/FEA Platform</h1>
      </header>
      
      <div className="content">
        <aside>
          <CreateSimulation onCreated={() => setRefreshKey(k => k + 1)} />
        </aside>
        
        <main>
          <SimulationList key={refreshKey} />
        </main>
      </div>
    </div>
  );
}