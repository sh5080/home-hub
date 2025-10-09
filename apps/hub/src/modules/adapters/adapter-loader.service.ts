import { Injectable, Inject, OnModuleInit, OnApplicationShutdown } from '@nestjs/common';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as yaml from 'js-yaml';
import type { Adapter } from '@home-hub/adapter-sdk';
import { DeviceRegistryService } from '../devices/device-registry.service';
import { loadConfig, getProjectRoot } from '../../config';
import { createLogger } from '../../logger';
import { DummyACAdapter } from '../../adapters/dummy-ac.adapter';

const ADAPTER_MAP: Record<string, () => Adapter> = {
  'dummy-ac': () => new DummyACAdapter(),
};

@Injectable()
export class AdapterLoaderService implements OnModuleInit, OnApplicationShutdown {
  private loadedAdapters: Adapter[] = [];

  constructor(
    @Inject(DeviceRegistryService)
    private readonly registry: DeviceRegistryService,
  ) {}

  async onModuleInit() {
    const config = loadConfig();
    const logger = createLogger(config.server.logLevel);
    const projectRoot = getProjectRoot();

    for (const adapterRef of config.adapters) {
      const factory = ADAPTER_MAP[adapterRef.id];
      if (!factory) {
        logger.warn({ id: adapterRef.id }, 'Unknown adapter, skipping');
        continue;
      }

      const adapter = factory();

      const configFilePath = path.resolve(projectRoot, adapterRef.configFile);
      let adapterConfig: unknown = {};
      if (fs.existsSync(configFilePath)) {
        adapterConfig = yaml.load(fs.readFileSync(configFilePath, 'utf-8'));
      }

      await adapter.init({
        logger: logger.child({ adapter: adapter.id }),
        config: adapterConfig,
      });

      this.registry.registerAdapter(adapter);

      const discovered = await adapter.discover();
      for (const d of discovered) {
        this.registry.registerDevice(
          {
            id: d.id,
            name: d.name,
            adapterId: adapter.id,
            type: d.type,
            capabilities: d.capabilities.map((c) => c.kind),
            online: true,
          },
          d.capabilities,
        );
      }

      this.loadedAdapters.push(adapter);
      logger.info(
        { adapterId: adapter.id, deviceCount: discovered.length },
        'Adapter loaded and devices registered',
      );
    }
  }

  async onApplicationShutdown() {
    for (const adapter of this.loadedAdapters) {
      await adapter.shutdown();
    }
  }
}
