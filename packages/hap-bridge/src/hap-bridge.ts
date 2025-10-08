import {
  Bridge,
  Accessory,
  Service,
  Characteristic,
  Categories,
  uuid,
} from 'hap-nodejs';
import type { Logger } from 'pino';

export interface HapBridgeConfig {
  bridgeName: string;
  pin: string;
  setupId: string;
  port: number;
  storagePath: string;
}

export class HapBridge {
  private bridge: Bridge;
  private readonly accessories: Map<string, Accessory> = new Map();

  constructor(
    private readonly config: HapBridgeConfig,
    private readonly logger: Logger,
  ) {
    const bridgeUuid = uuid.generate('hap-bridge:' + config.bridgeName);
    this.bridge = new Bridge(config.bridgeName, bridgeUuid);

    const info = this.bridge.getService(Service.AccessoryInformation)!;
    info.setCharacteristic(Characteristic.Manufacturer, 'Home Hub');
    info.setCharacteristic(Characteristic.Model, 'Hub Bridge v1');
    info.setCharacteristic(Characteristic.SerialNumber, 'HUB-001');
    info.setCharacteristic(Characteristic.FirmwareRevision, '0.1.0');
  }

  addAccessory(accessory: Accessory): void {
    this.bridge.addBridgedAccessory(accessory);
    this.accessories.set(accessory.displayName, accessory);
    this.logger.info({ name: accessory.displayName }, 'Accessory added to bridge');
  }

  async start(): Promise<void> {
    const { HAPStorage } = await import('hap-nodejs');
    HAPStorage.setCustomStoragePath(this.config.storagePath);

    this.bridge.publish({
      username: this.generateMac(),
      pincode: this.config.pin,
      setupID: this.config.setupId,
      port: this.config.port,
      category: Categories.BRIDGE,
    });

    this.logger.info(
      {
        bridgeName: this.config.bridgeName,
        pin: this.config.pin,
        port: this.config.port,
      },
      'HAP Bridge published — pair from iPhone Home app',
    );
  }

  async stop(): Promise<void> {
    this.bridge.unpublish();
    this.logger.info('HAP Bridge stopped');
  }

  private generateMac(): string {
    const hash = Buffer.from(this.config.bridgeName)
      .reduce((acc, b) => acc + b, 0);
    const hex = (n: number) => n.toString(16).padStart(2, '0').toUpperCase();
    return [
      hex(0xaa),
      hex(0xbb),
      hex((hash + 1) & 0xff),
      hex((hash + 2) & 0xff),
      hex((hash + 3) & 0xff),
      hex((hash + 4) & 0xff),
    ].join(':');
  }
}
