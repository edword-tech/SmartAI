import { Component, ErrorInfo, ReactNode } from 'react'
import { Result, Button } from 'antd'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('App error:', error, errorInfo)
  }

  retry = () => {
    this.setState({ hasError: false, error: null })
  }

  render() {
    if (this.state.hasError) {
      const isNetwork =
        this.state.error?.message?.includes('Network') ||
        this.state.error?.message?.includes('ECONNREFUSED') ||
        this.state.error?.message?.includes('Failed to fetch')
      return (
        <div style={{ padding: 48, maxWidth: 560, margin: '0 auto' }}>
          <Result
            status="warning"
            title={isNetwork ? '无法连接后端服务' : '页面加载异常'}
            subTitle={
              isNetwork
                ? '请先启动 Unified Resource Manager 后端（如 make run 或 go run ./cmd/main.go -port 8080），然后刷新本页。'
                : this.state.error?.message || '请刷新页面重试。'
            }
            extra={
              <Button type="primary" onClick={this.retry}>
                刷新重试
              </Button>
            }
          />
        </div>
      )
    }
    return this.props.children
  }
}
