CREATE TABLE `deviceStates` (
	`id` text PRIMARY KEY NOT NULL,
	`deviceId` text NOT NULL,
	`capability` text NOT NULL,
	`state` text NOT NULL,
	`source` text DEFAULT 'hub' NOT NULL,
	`createdAt` integer NOT NULL,
	FOREIGN KEY (`deviceId`) REFERENCES `devices`(`id`) ON UPDATE no action ON DELETE no action
);
--> statement-breakpoint
CREATE TABLE `devices` (
	`id` text PRIMARY KEY NOT NULL,
	`name` text NOT NULL,
	`adapterId` text NOT NULL,
	`type` text NOT NULL,
	`capabilities` text NOT NULL,
	`online` integer DEFAULT true NOT NULL,
	`createdAt` integer NOT NULL,
	`updatedAt` integer NOT NULL
);
--> statement-breakpoint
CREATE TABLE `events` (
	`id` text PRIMARY KEY NOT NULL,
	`type` text NOT NULL,
	`deviceId` text,
	`payload` text NOT NULL,
	`createdAt` integer NOT NULL
);
