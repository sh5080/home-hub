import { Injectable, Inject, OnModuleInit, OnApplicationShutdown } from '@nestjs/common';
import { HapBridge, type HapBridgeConfig } from '@home-hub/hap-bridge';
import { DeviceType } from '@home-hub/core';
import { DeviceRegistryService } from '../devices/device-registry.service';
import { loadConfig } from '../../config';
import { createLogger } from '../../logger';
import { createDummySwitch } from './dummy-switch';
import { createACAccessory } from './ac-accessory';
import type { Logger } from 'pino';

@Injectable()
export class HapService implements OnModuleInit, OnApplicationShutdown {
  private hapBridge!: HapBridge;
  private logger!: Logger;

  constructor(
    @Inject(DeviceRegistryService)
    private readonly registry: DeviceRegistryService,
  ) {}

  async onModuleInit() {
    const config = loadConfig();
    this.logger = createLogger(config.server.logLevel);

    const hapConfig: HapBridgeConfig = {
      bridgeName: config.hap.bridgeName,
      pin: config.hap.pin,
      setupId: config.hap.setupId,
      port: config.hap.port,
      storagePath: config.hap.storagePath,
    };

    this.hapBridge = new HapBridge(hapConfig, this.logger);

    const devices = this.registry.getAllDevices();
    for (const device of devices) {
      if (device.type === DeviceType.AC) {
        const accessory = createACAccessory(device, this.registry, this.logger);
        this.hapBridge.addAccessory(accessory);
      }
    }

    if (devices.length === 0) {
      const dummySwitch = createDummySwitch(this.logger);
      this.hapBridge.addAccessory(dummySwitch);
    }

    await this.hapBridge.start();
  }

  async onApplicationShutdown() {
    await this.hapBridge?.stop();
  }
}
