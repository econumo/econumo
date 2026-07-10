import { InfoBox } from 'web'

// Informational hint block used in settings sections (AccountsSettingsPage).
export const SettingsHint = () => (
  <div className="w-96">
    <InfoBox>Accounts are organized into folders. Hide a folder to keep accounts you rarely use out of the way.</InfoBox>
  </div>
)

export const ShortHint = () => (
  <div className="w-96">
    <InfoBox>Exchange rates update once a day.</InfoBox>
  </div>
)

export const MultiParagraph = () => (
  <div className="w-96">
    <InfoBox>
      <p>Econumo can run on your own server. Enter your server address to connect.</p>
      <p className="mt-1.5">Your data stays on your instance — nothing is sent to third parties.</p>
    </InfoBox>
  </div>
)
