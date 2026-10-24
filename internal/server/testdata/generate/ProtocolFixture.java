import com.hypixel.hytale.protocol.*;
import com.hypixel.hytale.protocol.packets.connection.*;
import com.hypixel.hytale.protocol.packets.auth.*;
import java.lang.foreign.MemorySegment;
import java.nio.file.*;
import java.util.*;
public class ProtocolFixture {
 public static void main(String[] args) throws Exception {
  String claims="{\"sub\":\"00112233-4455-6677-8899-aabbccddeeff\",\"username\":\"TestPlayer\"}";
  String token="eyJhbGciOiJub25lIn0."+Base64.getUrlEncoder().withoutPadding().encodeToString(claims.getBytes(java.nio.charset.StandardCharsets.UTF_8))+".synthetic";
  var packet=new Connect(ProtocolSettings.PROTOCOL_CRC,ProtocolSettings.PROTOCOL_BUILD_NUMBER,"0.7.4",ClientType.Game,token,"de-DE",new byte[]{1,2,3},new HostAddress("localhost",(short)5520));
  byte[] bytes=new byte[packet.computeSize()];packet.serialize(MemorySegment.ofArray(bytes),0);Files.write(Path.of(args[0]),bytes);
  var referral=new ClientReferral(new HostAddress("127.0.0.1",(short)15520),new byte[]{1,2,3});
  bytes=new byte[referral.computeSize()];referral.serialize(MemorySegment.ofArray(bytes),0);Files.write(Path.of(args[1]),bytes);
 }
}
