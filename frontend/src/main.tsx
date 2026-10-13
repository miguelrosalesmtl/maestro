import React from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import './styles/app.css'
import './styles/jira.css'
import App from './App'
import { ErrorBoundary } from './components/ErrorBoundary'
import { installGlobalErrorHandlers } from './lib/errors'

installGlobalErrorHandlers()

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>,
)
