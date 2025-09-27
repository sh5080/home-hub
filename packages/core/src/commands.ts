import type { CapabilityState } from './capabilities';

export enum CommandSource {
  HAP = 'hap',
  ADMIN = 'admin',
  AUTOMATION = 'automation',
}

export interface DeviceCommand {
  deviceId: string;
  capability: CapabilityState;
  source: CommandSource;
}
