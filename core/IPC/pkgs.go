package IPC

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// 长度
const MaxPayloadLen = 1024 * 1024

func GeneralPkg(data []byte) []byte {
	// 格式
	// [6F(起始标志),载荷长度(uint32, 大端序), 载荷, CRC32校验码]
	payloadLen := len(data)
	// 计算总长度:1(标头) + 4(长度) + payloadLen(载荷) + 4(CRC32)
	totalLen := 5 + payloadLen + 4
	pkg := make([]byte, totalLen)
	pkg[0] = 0x6F
	// 载荷长度 (4 bytes, uint32 大端序)
	binary.BigEndian.PutUint32(pkg[1:5], uint32(payloadLen))
	// 载荷 (payloadLen bytes)
	if payloadLen > 0 {
		copy(pkg[5:5+payloadLen], data)
	}
	// CRC32 校验码 (4 bytes)
	crcData := data
	crc := crc32.ChecksumIEEE(crcData)
	binary.BigEndian.PutUint32(pkg[5+payloadLen:], crc)
	return pkg
}

// 解析包
// ReadSinglePacket 从 reader 中读取并解析单个完整数据包
// 返回载荷数据和可能的错误
func ReadSinglePacket(reader io.Reader) ([]byte, error) {
	// 步骤1: 读取起始标志 0x6F
	header := make([]byte, 1)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("failed to read start marker: %w", err)
	}

	// 验证起始标志
	if header[0] != 0x6F {
		return nil, fmt.Errorf("invalid start marker: 0x%02X, expected 0x6F", header[0])
	}

	// 步骤2: 读取载荷长度 (4字节，大端序)
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, lenBuf); err != nil {
		return nil, fmt.Errorf("failed to read payload length: %w", err)
	}
	payloadLen := binary.BigEndian.Uint32(lenBuf)

	// 安全检查：防止异常长度
	if payloadLen > MaxPayloadLen {
		return nil, fmt.Errorf("payload length too large: %d, max allowed: %d", payloadLen, MaxPayloadLen)
	}

	// 步骤3: 读取载荷数据
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}
	}

	// 步骤4: 读取 CRC32 校验码 (4字节，大端序)
	crcBuf := make([]byte, 4)
	if _, err := io.ReadFull(reader, crcBuf); err != nil {
		return nil, fmt.Errorf("failed to read CRC: %w", err)
	}
	receivedCRC := binary.BigEndian.Uint32(crcBuf)

	// 步骤5: 验证 CRC32
	calculatedCRC := crc32.ChecksumIEEE(payload)
	if receivedCRC != calculatedCRC {
		return nil, fmt.Errorf("CRC mismatch: received 0x%08X, calculated 0x%08X", receivedCRC, calculatedCRC)
	}

	return payload, nil
}
