import { isPaywallEnabled as isPaywallEnabledConfig, getVersion } from './config';

interface EconumoPackage {
  label: string;
  includesConnections: boolean;
  includesSharedAccess: boolean;
  isPaywallEnabled: boolean;
  paywallUrl: string;
}

function getEditionLabel(): string {
  return getVersion();
}

function isPaywallEnabled(): boolean {
  return isPaywallEnabledConfig();
}

function getPaywallUrl(): string {
  return isPaywallEnabled() ? 'https://pay.econumo.com/cloud/' : '';
}

export const econumoPackage: EconumoPackage = {
  label: getEditionLabel(),
  includesConnections: true,
  includesSharedAccess: true,
  isPaywallEnabled: isPaywallEnabled(),
  paywallUrl: getPaywallUrl(),
};
