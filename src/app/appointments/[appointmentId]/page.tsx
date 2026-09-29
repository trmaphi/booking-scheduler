import Link from "next/link";

import { renderAppointmentPage } from "./appointment-view";

export default async function AppointmentPage({
  params,
}: {
  params: Promise<{ appointmentId: string }>;
}) {
  const { appointmentId } = await params;
  return (
    <main className="pageShell">
      <nav className="topbar" aria-label="Primary navigation">
        <Link className="brand" href="/" aria-label="Service Studio home">
          <span>SS</span> Service Studio
        </Link>
        <span className="secureNote">
          <i aria-hidden="true" /> Secure booking
        </span>
      </nav>
      <div className="bookingColumn">
        {await renderAppointmentPage({
          appointmentId,
          apiBaseURL: process.env.API_INTERNAL_BASE_URL ?? "",
        })}
      </div>
    </main>
  );
}
