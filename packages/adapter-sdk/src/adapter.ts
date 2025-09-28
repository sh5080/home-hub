import type { CapabilityState, DeviceCommand } from '@home-hub/core';
import { type DeviceType } from '@home-hub/core';
import type { Logger } from 'pino';

export interface DiscoveredDevice {
  id: string;
  name: string;
  type: DeviceType;
  capabilities: CapabilityState[];
}

export interface AdapterContext {
  logger: Logger;
  config: unknown;
}

export interface Adapter {
  readonly id: string;
  init(ctx: AdapterContext): Promise<void>;
  discover(): Promise<DiscoveredDevice[]>;
  execute(cmd: DeviceCommand): Promise<void>;
  getState(deviceId: string): Map<string, CapabilityState>;
  shutdown(): Promise<void>;
}
