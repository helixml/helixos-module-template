import "./globals.css";
import { HostProvider } from "@/components/host-provider";

export const metadata = { title: "Module" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <HostProvider>{children}</HostProvider>
      </body>
    </html>
  );
}
