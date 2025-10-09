import {
  BaseAdapter,
  type AdapterContext,
  type DiscoveredDevice,
} from '@home-hub/adapter-sdk';
import type {
  CapabilityState,
  DeviceCommand,
  OnOffState,
  ThermostatState,
  FanSpeedState,
} from '@home-hub/core';
import {
  CapabilityKind,
  ThermostatMode,
  TemperatureUnit,
  FanSpeedLevel,
  DeviceType,
} from '@home-hub/core';

interface DummyDeviceConfig {
  id: string;
  name: string;
  capabilities: string[];
}

interface DummyACConfig {
  devices: DummyDeviceConfig[];
}

export class DummyACAdapter extends BaseAdapter {
  readonly id = 'dummy-ac';
  private deviceStates = new Map<string, Map<string, CapabilityState>>();
  private devices: DummyDeviceConfig[] = [];

  override async init(ctx: AdapterContext): Promise<void> {
    await super.init(ctx);
    const cfg = ctx.config as DummyACConfig;
    this.devices = cfg?.devices ?? [];

    for (const device of this.devices) {
      const states = new Map<string, CapabilityState>();

      if (device.capabilities.includes(CapabilityKind.ON_OFF)) {
        states.set(CapabilityKind.ON_OFF, { kind: CapabilityKind.ON_OFF, value: false } satisfies OnOffState);
      }
      if (device.capabilities.includes(CapabilityKind.THERMOSTAT)) {
        states.set(CapabilityKind.THERMOSTAT, {
          kind: CapabilityKind.THERMOSTAT,
          mode: ThermostatMode.COOL,
          currentTemp: 26,
          targetTemp: 24,
          unit: TemperatureUnit.CELSIUS,
        } satisfies ThermostatState);
      }
      if (device.capabilities.includes(CapabilityKind.FAN_SPEED)) {
        states.set(CapabilityKind.FAN_SPEED, {
          kind: CapabilityKind.FAN_SPEED,
          level: FanSpeedLevel.AUTO,
        } satisfies FanSpeedState);
      }

      this.deviceStates.set(device.id, states);
    }

    this.logger.info({ deviceCount: this.devices.length }, 'Dummy AC devices loaded');
  }

  async discover(): Promise<DiscoveredDevice[]> {
    return this.devices.map((d) => ({
      id: d.id,
      name: d.name,
      type: DeviceType.AC,
      capabilities: Array.from(this.deviceStates.get(d.id)?.values() ?? []),
    }));
  }

  async execute(cmd: DeviceCommand): Promise<void> {
    const states = this.deviceStates.get(cmd.deviceId);
    if (!states) {
      this.logger.warn({ deviceId: cmd.deviceId }, 'Unknown device');
      return;
    }

    states.set(cmd.capability.kind, cmd.capability);
    this.logger.info(
      { deviceId: cmd.deviceId, capability: cmd.capability },
      'Dummy AC state updated',
    );
  }

  getState(deviceId: string): Map<string, CapabilityState> {
    return this.deviceStates.get(deviceId) ?? new Map();
  }
}
