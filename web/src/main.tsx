import { render } from 'preact';
import { App } from './app';
import './styles.css';

const root = document.getElementById('app');
if (!root) throw new Error('App root was not found');

render(<App />, root);

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js');
  });
}
