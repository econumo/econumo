import { Alert, AlertTitle, AlertDescription, AlertAction, Button } from 'web'
import { AlertCircle, Info, WifiOff } from 'lucide-react'

export const SessionExpired = () => (
  <div className="w-96">
    <Alert variant="destructive">
      <AlertCircle />
      <AlertDescription>Your session has expired. Please sign in again.</AlertDescription>
    </Alert>
  </div>
)

export const WithTitle = () => (
  <div className="w-96">
    <Alert>
      <Info />
      <AlertTitle>Exchange rates updated</AlertTitle>
      <AlertDescription>USD/EUR rates were refreshed 5 minutes ago.</AlertDescription>
    </Alert>
  </div>
)

export const WithAction = () => (
  <div className="w-96">
    <Alert variant="destructive">
      <WifiOff />
      <AlertTitle>Sync failed</AlertTitle>
      <AlertDescription>3 transactions could not be synced to the server.</AlertDescription>
      <AlertAction>
        <Button variant="outline" size="xs">
          Retry
        </Button>
      </AlertAction>
    </Alert>
  </div>
)
