import type { CapabilityState, DeviceCommand } from '@home-hub/core';
import type { Adapter, AdapterContext, DiscoveredDevice } from './adapter';
import type { Logger } from 'pino';

export abstract class BaseAdapter implements Adapter {
  abstract readonly id: string;
  protected logger!: Logger;
  protected config: unknown;

  async init(ctx: AdapterContext): Promise<void> {
    this.logger = ctx.logger.child({ adapter: this.id });
    this.config = ctx.config;
    this.logger.info('Adapter initialized');
  }

  abstract discover(): Promise<DiscoveredDevice[]>;
  abstract execute(cmd: DeviceCommand): Promise<void>;
  abstract getState(deviceId: string): Map<string, CapabilityState>;

  async shutdown(): Promise<void> {
    this.logger.info('Adapter shutting down');
  }
}
