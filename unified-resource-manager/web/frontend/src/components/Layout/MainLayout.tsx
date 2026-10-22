import { useState, useMemo, useEffect } from 'react'
import { Layout, Menu, theme, Breadcrumb } from 'antd'
import { useNavigate, useLocation } from 'react-router-dom'
import {
  DashboardOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  ClusterOutlined,
  LineChartOutlined,
  SettingOutlined,
  AlertOutlined,
  AppstoreOutlined,
  FolderOutlined,
  EyeOutlined,
  RocketOutlined,
  ControlOutlined,
  ApartmentOutlined,
} from '@ant-design/icons'

const { Header, Sider, Content } = Layout

const ROUTE_LABELS: Record<string, string> = {
  '/': '仪表板',
  '/architecture': '架构总览',
  '/pools': '资源池',
  '/cluster-pools': '集群大池',
  '/nodes': '节点管理',
  '/metrics': '监控指标',
  '/tasks': '任务管理',
  '/models': '模型市场',
  '/projects': '项目管理',
  '/compute/perception': '算力感知',
  '/compute/quality': '算力质量',
  '/compute/orchestration': '算力编排',
  '/compute/acceleration': '训练加速',
  '/scheduler': '智能调度',
  '/ops': '集群运维',
  '/settings': '系统配置',
}

function getBreadcrumbItems(pathname: string) {
  const segments = pathname.split('/').filter(Boolean)
  const items = [{ title: '首页', path: '/' }]
  let acc = ''
  for (const seg of segments) {
    acc += `/${seg}`
    const label = ROUTE_LABELS[acc] || seg
    items.push({ title: label, path: acc })
  }
  return items
}

interface MainLayoutProps {
  children: React.ReactNode
}

const MainLayout: React.FC<MainLayoutProps> = ({ children }) => {
  const [collapsed, setCollapsed] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const {
    token: { colorBgContainer },
  } = theme.useToken()

  const menuItems = useMemo(
    () => [
      {
        key: 'overview',
        icon: <DashboardOutlined />,
        label: '概览',
        children: [
          { key: '/', icon: <DashboardOutlined />, label: '仪表板' },
          { key: '/architecture', icon: <ApartmentOutlined />, label: '架构总览' },
        ],
      },
      {
        key: 'resource',
        icon: <DatabaseOutlined />,
        label: '资源与节点',
        children: [
          { key: '/pools', icon: <DatabaseOutlined />, label: '资源池' },
          { key: '/cluster-pools', icon: <ClusterOutlined />, label: '集群大池' },
          { key: '/nodes', icon: <ClusterOutlined />, label: '节点管理' },
          { key: '/metrics', icon: <LineChartOutlined />, label: '监控指标' },
        ],
      },
      {
        key: 'workload',
        icon: <ThunderboltOutlined />,
        label: '任务与模型',
        children: [
          { key: '/tasks', icon: <ThunderboltOutlined />, label: '任务管理' },
          { key: '/models', icon: <AppstoreOutlined />, label: '模型市场' },
          { key: '/projects', icon: <FolderOutlined />, label: '项目管理' },
        ],
      },
      {
        key: 'compute',
        icon: <ControlOutlined />,
        label: '算力',
        children: [
          { key: '/compute/perception', icon: <EyeOutlined />, label: '算力感知' },
          { key: '/compute/quality', icon: <AppstoreOutlined />, label: '算力质量' },
          { key: '/compute/orchestration', icon: <RocketOutlined />, label: '算力编排' },
          { key: '/compute/acceleration', icon: <ThunderboltOutlined />, label: '训练加速' },
          { key: '/scheduler', icon: <ThunderboltOutlined />, label: '智能调度' },
        ],
      },
      {
        key: 'ops',
        icon: <AlertOutlined />,
        label: '运维',
        children: [
          { key: '/ops', icon: <AlertOutlined />, label: '集群运维' },
        ],
      },
      {
        key: 'system',
        icon: <SettingOutlined />,
        label: '系统',
        children: [{ key: '/settings', icon: <SettingOutlined />, label: '系统配置' }],
      },
    ],
    []
  )

  const openKeysFromPath = useMemo(() => {
    const path = location.pathname
    if (path.startsWith('/compute') || path === '/scheduler') return ['compute']
    if (path === '/' || path === '' || path === '/architecture') return ['overview']
    if (['/pools', '/cluster-pools', '/nodes', '/metrics'].includes(path)) return ['resource']
    if (['/tasks', '/models', '/projects'].includes(path)) return ['workload']
    if (path === '/ops') return ['ops']
    if (path === '/settings') return ['system']
    return []
  }, [location.pathname])

  const [openKeys, setOpenKeys] = useState<string[]>(openKeysFromPath)
  useEffect(() => {
    setOpenKeys((prev) => (prev.length === 0 ? openKeysFromPath : [...new Set([...openKeysFromPath, ...prev])]))
  }, [openKeysFromPath.join(',')])
  const effectiveOpenKeys = collapsed ? undefined : (openKeys.length > 0 ? openKeys : openKeysFromPath)

  const breadcrumbItems = useMemo(() => getBreadcrumbItems(location.pathname), [location.pathname])

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        theme="dark"
        width={220}
        style={{
          overflow: 'auto',
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
        }}
      >
        <div
          style={{
            height: 64,
            margin: 16,
            background: 'linear-gradient(135deg, #1677ff 0%, #4096ff 100%)',
            borderRadius: 8,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'white',
            fontWeight: 'bold',
            fontSize: collapsed ? 14 : 15,
            boxShadow: '0 2px 8px rgba(0, 0, 0, 0.15)',
          }}
        >
          {collapsed ? 'Flex' : 'FlexAI'}
        </div>
        <Menu
          theme="dark"
          selectedKeys={[location.pathname || '/']}
          openKeys={effectiveOpenKeys}
          onOpenChange={setOpenKeys}
          mode="inline"
          items={menuItems}
          onClick={({ key }) => navigate(key)}
          style={{ borderRight: 0 }}
        />
      </Sider>
      <Layout style={{ marginLeft: collapsed ? 80 : 220, transition: 'margin-left 0.2s' }}>
        <Header
          style={{
            padding: '0 24px',
            background: colorBgContainer,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            boxShadow: '0 1px 4px rgba(0, 0, 0, 0.06)',
            position: 'sticky',
            top: 0,
            zIndex: 100,
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
            <h1 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>
              AI 超算与 GPU 池化 · 统一资源管理
            </h1>
            <Breadcrumb
              items={breadcrumbItems.map((item, i) => ({
                title: i < breadcrumbItems.length - 1 ? (
                  <a onClick={() => navigate(item.path)} style={{ cursor: 'pointer' }}>{item.title}</a>
                ) : (
                  item.title
                ),
              }))}
              style={{ fontSize: 13 }}
            />
          </div>
        </Header>
        <Content
          style={{
            margin: '24px',
            padding: 0,
            minHeight: 280,
            background: 'transparent',
          }}
        >
          {children}
        </Content>
      </Layout>
    </Layout>
  )
}

export default MainLayout
