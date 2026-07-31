import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { ReferenceApp } from './reference-app.jsx';
import './styles.css';
import './components/components.css';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <ReferenceApp />
  </StrictMode>,
);
