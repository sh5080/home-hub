import { Accessory, Service, Characteristic, uuid } from 'hap-nodejs';
import type { Logger } from 'pino';

export function createDummySwitch(logger: Logger): Accessory {
  const switchUuid = uuid.generate('dummy:switch:test');
  const accessory = new Accessory('Test Switch', switchUuid);

  const info = accessory.getService(Service.AccessoryInformation)!;
  info.setCharacteristic(Characteristic.Manufacturer, 'Home Hub');
  info.setCharacteristic(Characteristic.Model, 'Dummy Switch');
  info.setCharacteristic(Characteristic.SerialNumber, 'DUMMY-SW-001');

  let isOn = false;

  const switchService = accessory.addService(Service.Switch, 'Test Switch');

  switchService
    .getCharacteristic(Characteristic.On)
    .onGet(() => {
      logger.info({ isOn }, 'Switch state queried');
      return isOn;
    })
    .onSet((value) => {
      isOn = value as boolean;
      logger.info({ isOn }, 'Switch state changed');
    });

  return accessory;
}
