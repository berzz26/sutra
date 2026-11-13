import { Sparkles } from 'lucide-react'

export function HeroSection() {
  return (
    <section className="hero">
      {/* <div className="hero-pill">
        <span className="sparkle"><Sparkles size={13} /></span>
        Your video knowledge engine
      </div> */}
      <h1 className="hero-title">
        Remember the idea<br /><span className="accent">We'll find the moment</span>
      </h1>
      <p className="hero-description">Can't remember where something was discussed? Describe what you're looking for. Sutra searches your video transcripts and finds the relevant moment.</p>
    </section>
  )
}
