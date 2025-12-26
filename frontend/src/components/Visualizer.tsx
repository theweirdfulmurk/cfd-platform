import { useState, useEffect, useRef } from 'react';
import { visualizationAPI } from '../services/api';
import { Visualization } from '../types';
import './Visualizer.css';

interface VisualizerProps {
  simulationId: string;
  resultPath: string;
}

export function Visualizer({ simulationId, resultPath }: VisualizerProps) {
  const [viz, setViz] = useState<Visualization | null>(null);
  const [error, setError] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    createVisualization();
  }, [simulationId]);

  useEffect(() => {
    if (!viz) return;
    
    const interval = setInterval(async () => {
      try {
        const updated = await visualizationAPI.get(viz.id);
        setViz(updated);
        
        if (updated.status === 'ready' && !viz.webSocketURL) {
          const wsUrl = await visualizationAPI.getWebSocketURL(viz.id);
          setViz(v => v ? { ...v, webSocketURL: wsUrl } : null);
        }
      } catch (err) {
        console.error('Failed to update viz status', err);
      }
    }, 3000);

    return () => clearInterval(interval);
  }, [viz?.id]);

  async function createVisualization() {
    try {
      const newViz = await visualizationAPI.create(simulationId, resultPath);
      setViz(newViz);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create');
    }
  }

  if (error) return <div className="error">{error}</div>;
  if (!viz) return <div className="loading">Initializing...</div>;

  return (
    <div className="visualizer">
      <div className="viz-header">
        <h3>Visualization</h3>
        <span className={`status status-${viz.status}`}>{viz.status}</span>
      </div>
      
      {viz.status === 'ready' && viz.webSocketURL ? (
        <div className="viz-container" ref={containerRef}>
          <iframe
            src={`/pvw/visualizer.html?ws=${encodeURIComponent(viz.webSocketURL)}`}
            style={{ width: '100%', height: '600px', border: 'none' }}
          />
        </div>
      ) : (
        <div className="viz-loading">
          <p>Preparing visualization pod...</p>
          <p>Status: {viz.status}</p>
        </div>
      )}
    </div>
  );
}