import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import {StartupGate} from './StartupGate'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <StartupGate><App/></StartupGate>
    </React.StrictMode>
)
