import Link from "next/link";

import { renderAppointmentsPage } from "./appointments-view";

type Status = "CONFIRMED" | "CANCELLED";

export default async function AppointmentsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string | string[] }>;
}) {
  const rawStatus = (await searchParams).status;
  const status: Status | undefined =
    rawStatus === "CONFIRMED" || rawStatus === "CANCELLED"
      ? rawStatus
      : undefined;
  return (
    <main className="pageShell">
      <nav className="topbar" aria-label="Primary navigation">
        <Link className="brand" href="/" aria-label="Service Studio home">
          <span>SS</span> Service Studio
        </Link>
        <span className="secureNote">
          <i aria-hidden="true" /> Public schedule
        </span>
      </nav>
      {await renderAppointmentsPage({
        status,
        apiBaseURL: process.env.API_INTERNAL_BASE_URL ?? "",
      })}
    </main>
  );
}
