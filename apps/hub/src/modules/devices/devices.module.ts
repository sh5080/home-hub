import { Module, Global } from '@nestjs/common';
import { DeviceRegistryService } from './device-registry.service';

@Global()
@Module({
  providers: [DeviceRegistryService],
  exports: [DeviceRegistryService],
})
export class DevicesModule {}
