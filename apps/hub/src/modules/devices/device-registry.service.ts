import { Injectable } from '@nestjs/common';
import { EventEmitter2 } from '@nestjs/event-emitter';
import type {
  Device,
  DeviceWithState,
  CapabilityState,
  CapabilityKind,
  DeviceCommand,
  DeviceStateChangedEvent,
} from '@home-hub/core';
import { EventSource, HubEventType } from '@home-hub/core';
import type { Adapter } from '@home-hub/adapter-sdk';

@Injectable()
export class DeviceRegistryService {
  private devices = new Map<string, DeviceWithState>();
  private adapters = new Map<string, Adapter>();

  constructor(private readonly eventEmitter: EventEmitter2) {}

  registerAdapter(adapter: Adapter): void {
    this.adapters.set(adapter.id, adapter);
  }

  registerDevice(device: Device, initialStates: CapabilityState[]): void {
    const states = new Map<CapabilityKind, CapabilityState>();
    for (const s of initialStates) {
      states.set(s.kind, s);
    }
    this.devices.set(device.id, { ...device, states });
  }

  getDevice(deviceId: string): DeviceWithState | undefined {
    return this.devices.get(deviceId);
  }

  getAllDevices(): DeviceWithState[] {
    return Array.from(this.devices.values());
  }

  getAdapterForDevice(deviceId: string): Adapter | undefined {
    const device = this.devices.get(deviceId);
    if (!device) return undefined;
    return this.adapters.get(device.adapterId);
  }

  updateState(
    deviceId: string,
    newState: CapabilityState,
    source: EventSource,
  ): void {
    const device = this.devices.get(deviceId);
    if (!device) return;

    const previousState = device.states.get(newState.kind) ?? null;
    device.states.set(newState.kind, newState);

    const event: DeviceStateChangedEvent = {
      type: HubEventType.DEVICE_STATE_CHANGED,
      deviceId,
      capability: newState,
      previousState,
      source,
      timestamp: new Date(),
    };
    this.eventEmitter.emit(HubEventType.DEVICE_STATE_CHANGED, event);
  }

  async executeCommand(cmd: DeviceCommand): Promise<void> {
    const adapter = this.getAdapterForDevice(cmd.deviceId);
    if (!adapter) {
      throw new Error(`No adapter found for device ${cmd.deviceId}`);
    }

    await adapter.execute(cmd);
    this.updateState(cmd.deviceId, cmd.capability, EventSource.HUB);
  }
}
