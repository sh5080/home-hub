import { Module } from '@nestjs/common';
import { EventEmitterModule } from '@nestjs/event-emitter';
import { ScheduleModule } from '@nestjs/schedule';
import { DevicesModule } from './modules/devices/devices.module';
import { AdaptersModule } from './modules/adapters/adapters.module';
import { HapModule } from './modules/hap/hap.module';
import { AdminModule } from './modules/admin/admin.module';

@Module({
  imports: [
    EventEmitterModule.forRoot(),
    ScheduleModule.forRoot(),
    DevicesModule,
    AdaptersModule,
    HapModule,
    AdminModule,
  ],
})
export class AppModule {}
