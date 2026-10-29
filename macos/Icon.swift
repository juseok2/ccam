import AppKit
let output = CommandLine.arguments[1]
let sizes = [16, 32, 128, 256, 512]
try FileManager.default.createDirectory(atPath: output, withIntermediateDirectories: true)
for size in sizes {
    for scale in [1, 2] {
        let pixels = size * scale
        let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        let transform = NSAffineTransform()
        transform.scale(by: CGFloat(pixels) / 1024)
        transform.concat()
        let rect = NSRect(x: 60, y: 60, width: 904, height: 904)
        let background = NSBezierPath(roundedRect: rect, xRadius: 210, yRadius: 210)
        NSGradient(starting: NSColor(calibratedRed: 0.08, green: 0.76, blue: 0.72, alpha: 1), ending: NSColor(calibratedRed: 0.02, green: 0.36, blue: 0.48, alpha: 1))!.draw(in: background, angle: -75)
        NSColor.white.setStroke()
        func arrow(_ points: [NSPoint]) {
            let path = NSBezierPath()
            path.lineWidth = 65
            path.lineCapStyle = .round
            path.lineJoinStyle = .round
            path.move(to: points[0])
            for point in points.dropFirst() { path.line(to: point) }
            path.stroke()
        }
        arrow([NSPoint(x: 275, y: 655), NSPoint(x: 750, y: 655)])
        arrow([NSPoint(x: 630, y: 775), NSPoint(x: 750, y: 655), NSPoint(x: 630, y: 535)])
        arrow([NSPoint(x: 750, y: 370), NSPoint(x: 275, y: 370)])
        arrow([NSPoint(x: 395, y: 490), NSPoint(x: 275, y: 370), NSPoint(x: 395, y: 250)])
        NSGraphicsContext.restoreGraphicsState()
        let suffix = scale == 2 ? "@2x" : ""
        let url = URL(fileURLWithPath: output).appendingPathComponent("icon_\(size)x\(size)\(suffix).png")
        try rep.representation(using: .png, properties: [:])!.write(to: url)
    }
}
