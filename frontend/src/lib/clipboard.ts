// One copy-to-clipboard path for the whole panel.
//
// Two screens carried their own copy of this, and they had drifted: one awaited
// the Clipboard API and correctly fell through to the textarea when it was
// refused, the other fired the write, swallowed the rejection and returned
// success anyway, so its manual fallback could never run. They also disagreed
// about the last resort, and one of the two hardcoded its English text in a
// panel that ships twelve languages.
//
// The manual step is deliberately NOT here: showing text for the user to copy
// by hand is interface work, and the caller owns the dialog. This answers only
// whether the browser took the text.

/**
 * Copies text through whichever mechanism the browser allows.
 *
 * Returns false when every mechanism failed, so the caller can offer the text
 * for manual copying instead of reporting a success that did not happen.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  // The modern API needs a secure context, which rules out plain-HTTP panels.
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Permission denied or the document lost focus; the textarea still works.
    }
  }

  try {
    const field = document.createElement('textarea')
    field.value = text
    field.setAttribute('readonly', '')
    field.style.position = 'fixed'
    field.style.top = '0'
    field.style.left = '0'
    field.style.opacity = '0'
    document.body.appendChild(field)
    field.focus()
    field.select()
    field.setSelectionRange(0, text.length)
    const copied = document.execCommand('copy')
    document.body.removeChild(field)
    return copied
  } catch {
    return false
  }
}
