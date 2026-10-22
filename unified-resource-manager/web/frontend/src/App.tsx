import { BrowserRouter, Routes, Route } from 'react-router-dom'
import MainLayout from './components/Layout/MainLayout'
import Dashboard from './pages/Dashboard'
import ResourcePools from './pages/ResourcePools'
import Tasks from './pages/Tasks'
import Nodes from './pages/Nodes'
import Metrics from './pages/Metrics'
import Settings from './pages/Settings'
import Models from './pages/Models'
import Projects from './pages/Projects'
import ComputePerception from './pages/ComputePerception'
import ComputeQuality from './pages/ComputeQuality'
import ComputeOrchestration from './pages/ComputeOrchestration'
import TrainingAcceleration from './pages/TrainingAcceleration'
import ClusterPools from './pages/ClusterPools'
import Architecture from './pages/Architecture'
import SchedulerPage from './pages/Scheduler'
import OpsPage from './pages/Ops'

function App() {
  return (
    <BrowserRouter>
      <MainLayout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/architecture" element={<Architecture />} />
          <Route path="/pools" element={<ResourcePools />} />
          <Route path="/cluster-pools" element={<ClusterPools />} />
          <Route path="/tasks" element={<Tasks />} />
          <Route path="/nodes" element={<Nodes />} />
          <Route path="/metrics" element={<Metrics />} />
          <Route path="/models" element={<Models />} />
          <Route path="/projects" element={<Projects />} />
          <Route path="/compute/perception" element={<ComputePerception />} />
          <Route path="/compute/quality" element={<ComputeQuality />} />
          <Route path="/compute/orchestration" element={<ComputeOrchestration />} />
          <Route path="/compute/acceleration" element={<TrainingAcceleration />} />
          <Route path="/scheduler" element={<SchedulerPage />} />
          <Route path="/ops" element={<OpsPage />} />
          <Route path="/settings" element={<Settings />} />
        </Routes>
      </MainLayout>
    </BrowserRouter>
  )
}

export default App
