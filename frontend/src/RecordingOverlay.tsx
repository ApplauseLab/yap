import { useEffect, useRef, useState } from 'react';
import './RecordingOverlay.css';

interface RecordingOverlayProps {
  isRecording: boolean;
  onStop: () => void;
  onCancel: () => void;
}

export function RecordingOverlay({ isRecording, onStop, onCancel }: RecordingOverlayProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [waveformData, setWaveformData] = useState<number[]>([]);

  // Generate waveform animation
  useEffect(() => {
    if (!isRecording) {
      setWaveformData([]);
      return;
    }

    const interval = setInterval(() => {
      setWaveformData(prev => {
        const newData = [...prev];
        const sample = Math.random() * 0.4 + 0.3 + Math.sin(Date.now() / 200) * 0.2;
        newData.push(Math.max(0.15, Math.min(1, sample)));
        if (newData.length > 50) {
          newData.shift();
        }
        return newData;
      });
    }, 50);

    return () => clearInterval(interval);
  }, [isRecording]);

  // Draw waveform
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const width = canvas.width;
    const height = canvas.height;
    const centerY = height / 2;

    ctx.clearRect(0, 0, width, height);

    if (waveformData.length === 0) return;

    const barWidth = 3;
    const gap = 2;
    const totalBars = Math.floor(width / (barWidth + gap));
    const startIndex = Math.max(0, waveformData.length - totalBars);
    
    ctx.fillStyle = '#ffffff';
    
    for (let i = 0; i < totalBars; i++) {
      const dataIndex = startIndex + i;
      const amplitude = dataIndex < waveformData.length ? waveformData[dataIndex] : 0.15;
      const barHeight = Math.max(4, amplitude * (height - 20));
      const x = i * (barWidth + gap);
      const y = centerY - barHeight / 2;
      
      ctx.beginPath();
      ctx.roundRect(x, y, barWidth, barHeight, 1.5);
      ctx.fill();
    }
  }, [waveformData]);

  // Handle escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCancel();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onCancel]);

  if (!isRecording) return null;

  // Render directly without portal
  return (
    <div className="recording-overlay">
      <div className="recording-modal">
        <div className="waveform-container">
          <canvas 
            ref={canvasRef} 
            width={300} 
            height={60}
            className="waveform-canvas"
          />
        </div>
        
        <div className="recording-controls">
          <div className="recording-logo">
            <div className="logo-with-mic">
              <svg width="28" height="28" viewBox="0 0 130 130" fill="none">
                <rect x="11" y="11" width="108" height="108" fill="#00ff4e"/>
                <g fill="#000" opacity="0.9">
                  <rect x="55" y="35" width="20" height="35" rx="10"/>
                  <path d="M45 60v10a20 20 0 0040 0V60h-6v10a14 14 0 01-28 0V60h-6z"/>
                  <rect x="62" y="85" width="6" height="15"/>
                  <rect x="52" y="95" width="26" height="5" rx="2"/>
                </g>
              </svg>
            </div>
          </div>
          
          <div className="recording-actions">
            <button className="recording-btn stop-btn" onClick={onStop}>
              Stop
              <kbd>⌥</kbd>
            </button>
            <button className="recording-btn cancel-btn" onClick={onCancel}>
              Cancel
              <kbd>esc</kbd>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
