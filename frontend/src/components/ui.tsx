import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from 'react'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'outline'
type ButtonSize = 'sm' | 'md' | 'lg' | 'icon'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  children?: ReactNode
}

export function Button({ variant = 'primary', size = 'md', className = '', ...props }: ButtonProps) {
  return <button className={`ui-button button-${variant} button-${size} ${className}`} {...props} />
}

export function Input({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`ui-input ${className}`} {...props} />
}

export function Card({ className = '', children }: { className?: string; children: ReactNode }) {
  return <div className={`ui-card ${className}`}>{children}</div>
}

export function Badge({ tone = 'default', children }: { tone?: 'default' | 'accent' | 'success' | 'muted' | 'danger'; children: ReactNode }) {
  return <span className={`ui-badge badge-${tone}`}>{children}</span>
}

export function Separator() {
  return <div className="ui-separator" role="separator" />
}

export function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`ui-skeleton ${className}`} aria-hidden="true" />
}

export function Spinner() {
  return <span className="ui-spinner" aria-hidden="true" />
}

export function Dialog({ open, title, onClose, children }: { open: boolean; title: string; onClose: () => void; children: ReactNode }) {
  if (!open) return null
  return <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}><div className="dialog-panel dialog-wide" role="dialog" aria-modal="true" aria-labelledby="dialog-heading" onMouseDown={(event) => event.stopPropagation()}><div className="dialog-heading"><h2 id="dialog-heading">{title}</h2><Button variant="ghost" size="icon" aria-label="Close dialog" onClick={onClose}>×</Button></div>{children}</div></div>
}

interface AlertDialogProps {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  onCancel: () => void
  onConfirm: () => void
}

export function AlertDialog({ open, title, description, confirmLabel = 'Continue', onCancel, onConfirm }: AlertDialogProps) {
  if (!open) return null
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onCancel}>
      <div className="dialog-panel" role="alertdialog" aria-modal="true" aria-labelledby="dialog-title" onMouseDown={(event) => event.stopPropagation()}>
        <p className="eyebrow">Confirm action</p>
        <h2 id="dialog-title">{title}</h2>
        <p className="muted-copy">{description}</p>
        <div className="dialog-actions">
          <Button variant="ghost" onClick={onCancel}>Cancel</Button>
          <Button variant="danger" onClick={onConfirm}>{confirmLabel}</Button>
        </div>
      </div>
    </div>
  )
}
