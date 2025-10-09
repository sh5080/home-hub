import { Controller, Get, Param, Inject } from '@nestjs/common';
import { DeviceRegistryService } from '../devices/device-registry.service';

@Controller('devices')
export class AdminController {
  constructor(
    @Inject(DeviceRegistryService)
    private readonly registry: DeviceRegistryService,
  ) {}

  @Get()
  listDevices() {
    return this.registry.getAllDevices().map((d) => ({
      id: d.id,
      name: d.name,
      type: d.type,
      adapterId: d.adapterId,
      capabilities: d.capabilities,
      online: d.online,
      states: Object.fromEntries(d.states),
    }));
  }

  @Get(':id')
  getDevice(@Param('id') id: string) {
    const device = this.registry.getDevice(id);
    if (!device) {
      return { error: 'Device not found' };
    }
    return {
      id: device.id,
      name: device.name,
      type: device.type,
      adapterId: device.adapterId,
      capabilities: device.capabilities,
      online: device.online,
      states: Object.fromEntries(device.states),
    };
  }

  @Get(':id/states')
  getDeviceStates(@Param('id') id: string) {
    const device = this.registry.getDevice(id);
    if (!device) {
      return { error: 'Device not found' };
    }
    return Object.fromEntries(device.states);
  }
}
