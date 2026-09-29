import { BookingExperience } from "@/features/booking/ui/booking-experience";

export default function Home() {
  return (
    <main className="pageShell">
      <nav className="topbar" aria-label="Primary navigation">
        <a className="brand" href="#top" aria-label="Service Studio home">
          <span>SS</span> Service Studio
        </a>
        <span className="secureNote">
          <i aria-hidden="true" /> Secure booking
        </span>
      </nav>
      <div className="hero" id="top">
        <section className="heroCopy">
          <p className="kicker">Care for every journey</p>
          <h1>Book your next service with confidence.</h1>
          <p>
            Choose a time that works for you. We’ll coordinate the right
            specialist and service bay behind the scenes.
          </p>
          <div className="benefits" aria-label="Booking benefits">
            <span>
              <b>01</b> Live availability
            </span>
            <span>
              <b>02</b> Qualified specialists
            </span>
            <span>
              <b>03</b> Instant confirmation
            </span>
          </div>
        </section>
        <div className="bookingColumn">
          <BookingExperience />
        </div>
      </div>
      <footer>
        <span>Service Studio</span>
        <span>Transparent scheduling, built around your time.</span>
      </footer>
    </main>
  );
}
