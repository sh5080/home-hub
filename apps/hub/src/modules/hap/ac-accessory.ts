import { Accessory, Service, Characteristic, uuid } from 'hap-nodejs';
import type { Logger } from 'pino';
import type {
  DeviceWithState,
  OnOffState,
  ThermostatState,
  FanSpeedState,
} from '@home-hub/core';
import {
  CapabilityKind,
  ThermostatMode,
  TemperatureUnit,
  FanSpeedLevel,
  CommandSource,
} from '@home-hub/core';
import type { DeviceRegistryService } from '../devices/device-registry.service';

const MODE_TO_HAP: Record<ThermostatMode, number> = {
  [ThermostatMode.COOL]: Characteristic.TargetHeaterCoolerState.COOL,
  [ThermostatMode.HEAT]: Characteristic.TargetHeaterCoolerState.HEAT,
  [ThermostatMode.AUTO]: Characteristic.TargetHeaterCoolerState.AUTO,
  [ThermostatMode.FAN]: Characteristic.TargetHeaterCoolerState.AUTO,
  [ThermostatMode.DRY]: Characteristic.TargetHeaterCoolerState.AUTO,
  [ThermostatMode.OFF]: Characteristic.TargetHeaterCoolerState.AUTO,
};

const HAP_TO_MODE: Record<number, ThermostatMode> = {
  [Characteristic.TargetHeaterCoolerState.COOL]: ThermostatMode.COOL,
  [Characteristic.TargetHeaterCoolerState.HEAT]: ThermostatMode.HEAT,
  [Characteristic.TargetHeaterCoolerState.AUTO]: ThermostatMode.AUTO,
};

const FAN_LEVEL_TO_PERCENT: Record<FanSpeedLevel, number> = {
  [FanSpeedLevel.AUTO]: 0,
  [FanSpeedLevel.LOW]: 33,
  [FanSpeedLevel.MID]: 66,
  [FanSpeedLevel.HIGH]: 100,
};

function percentToFanLevel(percent: number): FanSpeedLevel {
  if (percent <= 10) return FanSpeedLevel.AUTO;
  if (percent <= 45) return FanSpeedLevel.LOW;
  if (percent <= 78) return FanSpeedLevel.MID;
  return FanSpeedLevel.HIGH;
}

export function createACAccessory(
  device: DeviceWithState,
  registry: DeviceRegistryService,
  logger: Logger,
): Accessory {
  const log = logger.child({ device: device.id });
  const accessoryUuid = uuid.generate('home-hub:ac:' + device.id);
  const accessory = new Accessory(device.name, accessoryUuid);

  const info = accessory.getService(Service.AccessoryInformation)!;
  info.setCharacteristic(Characteristic.Manufacturer, 'Home Hub');
  info.setCharacteristic(Characteristic.Model, 'AC via ' + device.adapterId);
  info.setCharacteristic(Characteristic.SerialNumber, device.id);

  const heaterCooler = accessory.addService(Service.HeaterCooler, device.name);

  // --- Active (on/off) ---
  heaterCooler
    .getCharacteristic(Characteristic.Active)
    .onGet(() => {
      const state = device.states.get(CapabilityKind.ON_OFF) as OnOffState | undefined;
      return state?.value
        ? Characteristic.Active.ACTIVE
        : Characteristic.Active.INACTIVE;
    })
    .onSet((value) => {
      const isOn = value === Characteristic.Active.ACTIVE;
      log.info({ isOn }, 'HAP → onOff');
      registry.executeCommand({
        deviceId: device.id,
        capability: { kind: CapabilityKind.ON_OFF, value: isOn },
        source: CommandSource.HAP,
      });
    });

  // --- Current Heater Cooler State ---
  heaterCooler
    .getCharacteristic(Characteristic.CurrentHeaterCoolerState)
    .onGet(() => {
      const onOff = device.states.get(CapabilityKind.ON_OFF) as OnOffState | undefined;
      if (!onOff?.value) return Characteristic.CurrentHeaterCoolerState.INACTIVE;

      const therm = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      if (!therm) return Characteristic.CurrentHeaterCoolerState.IDLE;

      switch (therm.mode) {
        case ThermostatMode.COOL:
        case ThermostatMode.DRY:
          return Characteristic.CurrentHeaterCoolerState.COOLING;
        case ThermostatMode.HEAT:
          return Characteristic.CurrentHeaterCoolerState.HEATING;
        default:
          return Characteristic.CurrentHeaterCoolerState.IDLE;
      }
    });

  // --- Target Heater Cooler State (mode) ---
  heaterCooler
    .getCharacteristic(Characteristic.TargetHeaterCoolerState)
    .onGet(() => {
      const therm = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      return MODE_TO_HAP[therm?.mode ?? ThermostatMode.AUTO];
    })
    .onSet((value) => {
      const mode = HAP_TO_MODE[value as number] ?? ThermostatMode.AUTO;
      const current = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      log.info({ mode }, 'HAP → thermostat mode');
      registry.executeCommand({
        deviceId: device.id,
        capability: {
          kind: CapabilityKind.THERMOSTAT,
          mode,
          currentTemp: current?.currentTemp ?? 26,
          targetTemp: current?.targetTemp ?? 24,
          unit: TemperatureUnit.CELSIUS,
        },
        source: CommandSource.HAP,
      });
    });

  // --- Current Temperature ---
  heaterCooler
    .getCharacteristic(Characteristic.CurrentTemperature)
    .onGet(() => {
      const therm = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      return therm?.currentTemp ?? 26;
    });

  // --- Cooling Threshold Temperature ---
  heaterCooler
    .getCharacteristic(Characteristic.CoolingThresholdTemperature)
    .setProps({ minValue: 16, maxValue: 30, minStep: 1 })
    .onGet(() => {
      const therm = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      return therm?.targetTemp ?? 24;
    })
    .onSet((value) => {
      const temp = value as number;
      const current = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      log.info({ targetTemp: temp }, 'HAP → cooling threshold');
      registry.executeCommand({
        deviceId: device.id,
        capability: {
          kind: CapabilityKind.THERMOSTAT,
          mode: current?.mode ?? ThermostatMode.COOL,
          currentTemp: current?.currentTemp ?? 26,
          targetTemp: temp,
          unit: TemperatureUnit.CELSIUS,
        },
        source: CommandSource.HAP,
      });
    });

  // --- Heating Threshold Temperature ---
  heaterCooler
    .getCharacteristic(Characteristic.HeatingThresholdTemperature)
    .setProps({ minValue: 16, maxValue: 30, minStep: 1 })
    .onGet(() => {
      const therm = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      return therm?.targetTemp ?? 24;
    })
    .onSet((value) => {
      const temp = value as number;
      const current = device.states.get(CapabilityKind.THERMOSTAT) as ThermostatState | undefined;
      log.info({ targetTemp: temp }, 'HAP → heating threshold');
      registry.executeCommand({
        deviceId: device.id,
        capability: {
          kind: CapabilityKind.THERMOSTAT,
          mode: current?.mode ?? ThermostatMode.HEAT,
          currentTemp: current?.currentTemp ?? 26,
          targetTemp: temp,
          unit: TemperatureUnit.CELSIUS,
        },
        source: CommandSource.HAP,
      });
    });

  // --- Fan Speed (Rotation Speed characteristic) ---
  if (device.capabilities.includes(CapabilityKind.FAN_SPEED)) {
    heaterCooler
      .addCharacteristic(Characteristic.RotationSpeed)
      .setProps({ minValue: 0, maxValue: 100, minStep: 1 })
      .onGet(() => {
        const fan = device.states.get(CapabilityKind.FAN_SPEED) as FanSpeedState | undefined;
        return FAN_LEVEL_TO_PERCENT[fan?.level ?? FanSpeedLevel.AUTO];
      })
      .onSet((value) => {
        const level = percentToFanLevel(value as number);
        log.info({ level, percent: value }, 'HAP → fanSpeed');
        registry.executeCommand({
          deviceId: device.id,
          capability: { kind: CapabilityKind.FAN_SPEED, level },
          source: CommandSource.HAP,
        });
      });
  }

  return accessory;
}
