import { Sparkles } from 'lucide-react'

export function HeroSection() {
  return (
    <section className="hero">
      <div className="hero-pill">
        <span className="sparkle"><Sparkles size={13} /></span>
        Your video knowledge engine
      </div>
      <h1 className="hero-title">
        Find the moment<br /><span className="accent">that matters.</span>
      </h1>
      <p className="hero-description">Search every video, lecture, and idea in your library using natural language.</p>
    </section>
  )
}
