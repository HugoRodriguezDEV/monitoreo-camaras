import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import Admin from './Admin'
import Instalar from './Instalar'
import './gestion.css'

const ruta = location.pathname.replace(/\/+$/, '')
const Pantalla = ruta === '/admin' ? Admin : ruta === '/instalar' ? Instalar : App

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Pantalla />
  </StrictMode>,
)
