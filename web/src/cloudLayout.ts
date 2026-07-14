import cloud from 'd3-cloud'

export interface CloudWord {
  text: string
  size: number
  kind: string
}

export interface PlacedWord extends CloudWord {
  x: number
  y: number
}

// layoutCloud runs the d3-cloud (Wordle) packing. Needs canvas for text
// measurement, so component tests mock this module. Words that don't fit the
// box are silently dropped by d3-cloud — acceptable for the tail of the top-80.
export function layoutCloud(words: CloudWord[], width: number, height: number): Promise<PlacedWord[]> {
  return new Promise((resolve) => {
    cloud()
      .size([width, height])
      .words(words.map((w) => ({ ...w })))
      .padding(3)
      .rotate(0)
      .font('system-ui')
      .fontSize((d) => d.size ?? 12)
      .on('end', (out) => resolve(out as unknown as PlacedWord[]))
      .start()
  })
}
