export type AppointmentStatus = "CONFIRMED" | "CANCELLED";

export interface AppointmentInterval {
  startAt: string;
  endAt: string;
}

export interface ResourceAssignment {
  technicianId: string;
  serviceBayId: string;
}
